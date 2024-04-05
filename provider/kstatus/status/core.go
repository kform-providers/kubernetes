// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"
	"math"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetConditionsFn defines the signature for functions to compute the
// status of a built-in resource.
type GetConditionsFn func(*unstructured.Unstructured) (*Result, error)

// legacyTypes defines the mapping from GroupKind to a function that can
// compute the status for the given resource.
var legacyTypes = map[string]GetConditionsFn{
	"Service":                    serviceConditions,
	"Pod":                        podConditions,
	"Secret":                     alwaysReady,
	"PersistentVolumeClaim":      pvcConditions,
	"apps/StatefulSet":           stsConditions,
	"apps/DaemonSet":             daemonsetConditions,
	"extensions/DaemonSet":       daemonsetConditions,
	"apps/Deployment":            deploymentConditions,
	"extensions/Deployment":      deploymentConditions,
	"apps/ReplicaSet":            replicasetConditions,
	"extensions/ReplicaSet":      replicasetConditions,
	"policy/PodDisruptionBudget": pdbConditions,
	"batch/CronJob":              alwaysReady,
	"ConfigMap":                  alwaysReady,
	"batch/Job":                  jobConditions,
	"apiextensions.k8s.io/CustomResourceDefinition": crdConditions,
}

const (
	//tooFewReady     = "LessReady"
	//tooFewAvailable = "LessAvailable"
	//tooFewUpdated   = "LessUpdated"
	//tooFewReplicas  = "LessReplicas"
	//extraPods       = "ExtraPods"

	onDeleteUpdateStrategy = "OnDelete"

	// How long a pod can be unscheduled before it is reported as
	// unschedulable.
	ScheduleWindow = 15 * time.Second
)

// GetLegacyConditionsFn returns a function that can compute the status for the
// given resource, or nil if the resource type is not known.
func GetLegacyConditionsFn(u *unstructured.Unstructured) GetConditionsFn {
	gvk := u.GroupVersionKind()
	g := gvk.Group
	k := gvk.Kind
	key := g + "/" + k
	if g == "" {
		key = k
	}
	return legacyTypes[key]
}

// alwaysReady Used for resources that are always ready
func alwaysReady(u *unstructured.Unstructured) (*Result, error) {
	return ready("ready"), nil
}

// stsConditions return standardized Conditions for Statefulset
//
// StatefulSet does define the .status.conditions property, but the controller never
// actually sets any Conditions. Thus, status must be computed only based on the other
// properties under .status. We don't have any way to find out if a reconcile for a
// StatefulSet has failed.
func stsConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// updateStrategy==ondelete is a user managed statefulset.
	updateStrategy := GetStringField(obj, ".spec.updateStrategy.type", "")
	if updateStrategy == onDeleteUpdateStrategy {
		return userManaged(), nil
	}

	// Replicas
	specReplicas := GetIntField(obj, ".spec.replicas", 1)
	readyReplicas := GetIntField(obj, ".status.readyReplicas", 0)
	currentReplicas := GetIntField(obj, ".status.currentReplicas", 0)
	updatedReplicas := GetIntField(obj, ".status.updatedReplicas", 0)
	statusReplicas := GetIntField(obj, ".status.replicas", 0)
	partition := GetIntField(obj, ".spec.updateStrategy.rollingUpdate.partition", -1)

	if specReplicas > statusReplicas {
		msg := fmt.Sprintf("Replicas: %d/%d", statusReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if specReplicas > readyReplicas {
		msg := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if statusReplicas > specReplicas {
		msg := fmt.Sprintf("Pending termination: %d", statusReplicas-specReplicas)
		return inProgress(msg), nil
	}

	// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#partitions
	if partition != -1 {
		if updatedReplicas < (specReplicas - partition) {
			msg := fmt.Sprintf("updated: %d/%d", updatedReplicas, specReplicas-partition)
			return inProgress(msg), nil
		}
		// Partition case All ok
		msg := fmt.Sprintf("Partition rollout complete. updated: %d", updatedReplicas)
		return ready(msg), nil
	}

	if specReplicas > currentReplicas {
		msg := fmt.Sprintf("current: %d/%d", currentReplicas, specReplicas)
		return inProgress(msg), nil
	}

	// Revision
	currentRevision := GetStringField(obj, ".status.currentRevision", "")
	updatedRevision := GetStringField(obj, ".status.updateRevision", "")
	if currentRevision != updatedRevision {
		msg := "Waiting for updated revision to match current"
		return inProgress(msg), nil
	}

	// All ok
	msg := fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", statusReplicas)
	return ready(msg), nil
}

// deploymentConditions return standardized Conditions for Deployment.
//
// For Deployments, we look at .status.conditions as well as the other properties
// under .status. Status will be Failed if the progress deadline has been exceeded.
func deploymentConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	progressing := false

	// Check if progressDeadlineSeconds is set. If not, the controller will not set
	// the `Progressing` condition, so it will always consider a deployment to be
	// progressing. The use of math.MaxInt32 is due to special handling in the
	// controller:
	// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/util/deployment_util.go#L886
	progressDeadline := GetIntField(obj, ".spec.progressDeadlineSeconds", math.MaxInt32)
	if progressDeadline == math.MaxInt32 {
		progressing = true
	}

	available := false

	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}

	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Progressing": // appsv1.DeploymentProgressing:
			// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/progress.go#L52
			if c.Reason == "ProgressDeadlineExceeded" {
				return failed(c.Message), nil
			}
			if c.Status == metav1.ConditionTrue && c.Reason == "NewReplicaSetAvailable" {
				progressing = true
			}
		case "Available": // appsv1.DeploymentAvailable:
			if c.Status == metav1.ConditionTrue {
				available = true
			}
		}
	}

	// replicas
	specReplicas := GetIntField(obj, ".spec.replicas", 1) // Controller uses 1 as default if not specified.
	statusReplicas := GetIntField(obj, ".status.replicas", 0)
	updatedReplicas := GetIntField(obj, ".status.updatedReplicas", 0)
	readyReplicas := GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := GetIntField(obj, ".status.availableReplicas", 0)

	// TODO spec.replicas zero case ??

	if specReplicas > statusReplicas {
		msg := fmt.Sprintf("Replicas: %d/%d", statusReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if specReplicas > updatedReplicas {
		msg := fmt.Sprintf("Updated: %d/%d", updatedReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if statusReplicas > specReplicas {
		msg := fmt.Sprintf("Pending termination: %d", statusReplicas-specReplicas)
		return inProgress(msg), nil
	}

	if updatedReplicas > availableReplicas {
		msg := fmt.Sprintf("Available: %d/%d", availableReplicas, updatedReplicas)
		return inProgress(msg), nil
	}

	if specReplicas > readyReplicas {
		msg := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return inProgress(msg), nil
	}

	// check conditions
	if !progressing {
		msg := "ReplicaSet not Available"
		return inProgress(msg), nil
	}
	if !available {
		msg := "Deployment not Available"
		return inProgress(msg), nil
	}
	// All ok
	msg := fmt.Sprintf("Deployment is available. Replicas: %d", statusReplicas)
	return ready(msg), nil
}

// replicasetConditions return standardized Conditions for Replicaset
func replicasetConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// Conditions
	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}

	for _, c := range objc.Status.Conditions {
		// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/replicaset/replica_set_utils.go
		if c.Type == "ReplicaFailure" && c.Status == metav1.ConditionTrue {
			msg := "Replica Failure condition. Check Pods"
			return inProgress(msg), nil
		}
	}

	// Replicas
	specReplicas := GetIntField(obj, ".spec.replicas", 1) // Controller uses 1 as default if not specified.
	statusReplicas := GetIntField(obj, ".status.replicas", 0)
	readyReplicas := GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := GetIntField(obj, ".status.availableReplicas", 0)
	fullyLabelledReplicas := GetIntField(obj, ".status.fullyLabeledReplicas", 0)

	if specReplicas > fullyLabelledReplicas {
		msg := fmt.Sprintf("Labelled: %d/%d", fullyLabelledReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if specReplicas > availableReplicas {
		msg := fmt.Sprintf("Available: %d/%d", availableReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if specReplicas > readyReplicas {
		msg := fmt.Sprintf("Ready: %d/%d", readyReplicas, specReplicas)
		return inProgress(msg), nil
	}

	if statusReplicas > specReplicas {
		msg := fmt.Sprintf("Pending termination: %d", statusReplicas-specReplicas)
		return inProgress(msg), nil
	}
	// All ok
	msg := fmt.Sprintf("ReplicaSet is available. Replicas: %d", statusReplicas)
	return ready(msg), nil
}

// daemonsetConditions return standardized Conditions for DaemonSet
func daemonsetConditions(u *unstructured.Unstructured) (*Result, error) {
	// We check that the latest generation is equal to observed generation as
	// part of checking generic properties but in that case, we are lenient and
	// skip the check if those fields are unset. For daemonset, we know that if
	// the daemonset controller has acted on a resource, these fields would not
	// be unset. So, we ensure that here.
	res, err := checkGenerationSet(u)
	if err != nil || res != nil {
		return res, err
	}

	obj := u.UnstructuredContent()

	// replicas
	desiredNumberScheduled := GetIntField(obj, ".status.desiredNumberScheduled", -1)
	currentNumberScheduled := GetIntField(obj, ".status.currentNumberScheduled", 0)
	updatedNumberScheduled := GetIntField(obj, ".status.updatedNumberScheduled", 0)
	numberAvailable := GetIntField(obj, ".status.numberAvailable", 0)
	numberReady := GetIntField(obj, ".status.numberReady", 0)

	if desiredNumberScheduled == -1 {
		msg := "Missing .status.desiredNumberScheduled"
		return inProgress(msg), nil
	}

	if desiredNumberScheduled > currentNumberScheduled {
		msg := fmt.Sprintf("Current: %d/%d", currentNumberScheduled, desiredNumberScheduled)
		return inProgress(msg), nil
	}

	if desiredNumberScheduled > updatedNumberScheduled {
		msg := fmt.Sprintf("Updated: %d/%d", updatedNumberScheduled, desiredNumberScheduled)
		return inProgress(msg), nil
	}

	if desiredNumberScheduled > numberAvailable {
		msg := fmt.Sprintf("Available: %d/%d", numberAvailable, desiredNumberScheduled)
		return inProgress(msg), nil
	}

	if desiredNumberScheduled > numberReady {
		msg := fmt.Sprintf("Ready: %d/%d", numberReady, desiredNumberScheduled)
		return inProgress(msg), nil
	}

	// All ok
	msg := fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", desiredNumberScheduled)
	return ready(msg), nil
}

// checkGenerationSet checks that the metadata.generation and
// status.observedGeneration fields are set.
func checkGenerationSet(u *unstructured.Unstructured) (*Result, error) {
	_, found, err := unstructured.NestedInt64(u.Object, "metadata", "generation")
	if err != nil {
		return nil, fmt.Errorf("looking up metadata.generation from resource: %w", err)
	}
	if !found {
		msg := fmt.Sprintf("%s metadata.generation not found", u.GetKind())
		return inProgress(msg), nil
	}

	_, found, err = unstructured.NestedInt64(u.Object, "status", "observedGeneration")
	if err != nil {
		return nil, fmt.Errorf("looking up status.observedGeneration from resource: %w", err)
	}
	if !found {
		msg := fmt.Sprintf("%s status.observedGeneration not found", u.GetKind())
		return inProgress(msg), nil
	}
	return nil, nil
}

// pvcConditions return standardized Conditions for PVC
func pvcConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	phase := GetStringField(obj, ".status.phase", "unknown")
	if phase != "Bound" { // corev1.ClaimBound
		msg := fmt.Sprintf("PVC is not Bound. phase: %s", phase)
		return inProgress(msg), nil
	}
	// All ok
	return ready("PVC is bound"), nil
}

// podConditions return standardized Conditions for Pod
func podConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()
	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}
	phase := GetStringField(obj, ".status.phase", "")

	switch phase {
	case "Succeeded":
		return ready("Pod completed"), nil
	case "Failed":
		return failed("Pod failed"), nil
	case "Running":
		if hasConditionWithStatus(objc.Status.Conditions, "Ready", metav1.ConditionTrue) {
			return ready("Pod ready"), nil
		}

		containerNames, isCrashLooping, err := getCrashLoopingContainers(obj)
		if err != nil {
			return nil, err
		}
		if isCrashLooping {
			msg := fmt.Sprintf("Containers in CrashLoop state: %s", strings.Join(containerNames, ","))
			return failed(msg), nil
		}

		msg := "Pod is running but is not Ready"
		return inProgress(msg), nil
	case "Pending":
		c, found := getConditionWithStatus(objc.Status.Conditions, "PodScheduled", metav1.ConditionFalse)
		if found && c.Reason == "Unschedulable" {
			if time.Now().Add(-ScheduleWindow).Before(u.GetCreationTimestamp().Time) {
				// We give the pod 15 seconds to be scheduled before we report it
				// as unschedulable.
				msg := "Pod has not been scheduled"
				return inProgress(msg), nil
			}
			msg := "Pod could not be scheduled"
			return failed(msg), nil
		}
		msg := "Pod is in the Pending phase"
		return inProgress(msg), nil
	default:
		// If the controller hasn't observed the pod yet, there is no phase. We consider this as it
		// still being in progress.
		if phase == "" {
			msg := "Pod phase not available"
			return inProgress(msg), nil
		}
		return nil, fmt.Errorf("unknown phase %s", phase)
	}
}

func getCrashLoopingContainers(obj map[string]interface{}) ([]string, bool, error) {
	var containerNames []string
	css, found, err := unstructured.NestedSlice(obj, "status", "containerStatuses")
	if !found || err != nil {
		return containerNames, found, err
	}
	for _, item := range css {
		cs := item.(map[string]interface{})
		n, found := cs["name"]
		if !found {
			continue
		}
		name := n.(string)
		s, found := cs["state"]
		if !found {
			continue
		}
		state := s.(map[string]interface{})

		ws, found := state["waiting"]
		if !found {
			continue
		}
		waitingState := ws.(map[string]interface{})

		r, found := waitingState["reason"]
		if !found {
			continue
		}
		reason := r.(string)
		if reason == "CrashLoopBackOff" {
			containerNames = append(containerNames, name)
		}
	}
	if len(containerNames) > 0 {
		return containerNames, true, nil
	}
	return containerNames, false, nil
}

// pdbConditions computes the status for PodDisruptionBudgets. A PDB
// is currently considered Current if the disruption controller has
// observed the latest version of the PDB resource and has computed
// the AllowedDisruptions. PDBs do have ObservedGeneration in the
// Status object, so if this function gets called we know that
// the controller has observed the latest changes.
// The disruption controller does not set any conditions if
// computing the AllowedDisruptions fails (and there are many ways
// it can fail), but there is PR against OSS Kubernetes to address
// this: https://github.com/kubernetes/kubernetes/pull/86929
func pdbConditions(_ *unstructured.Unstructured) (*Result, error) {
	// All ok
	return ready("AllowedDisruptions has been computed."), nil
}

// jobConditions return standardized Conditions for Job
//
// A job will have the InProgress status until it starts running. Then it will have the Current
// status while the job is running and after it has been completed successfully. It
// will have the Failed status if it the job has failed.
func jobConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	parallelism := GetIntField(obj, ".spec.parallelism", 1)
	completions := GetIntField(obj, ".spec.completions", parallelism)
	succeeded := GetIntField(obj, ".status.succeeded", 0)
	active := GetIntField(obj, ".status.active", 0)
	podFailed := GetIntField(obj, ".status.failed", 0)
	starttime := GetStringField(obj, ".status.startTime", "")

	// Conditions
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/job/utils.go#L24
	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Complete":
			if c.Status == metav1.ConditionTrue {
				msg := fmt.Sprintf("Job Completed. succeeded: %d/%d", succeeded, completions)
				return ready(msg), nil
			}
		case "Failed":
			if c.Status == metav1.ConditionTrue {
				msg := fmt.Sprintf("Job Failed. failed: %d/%d", podFailed, completions)
				return failed(msg), nil
			}
		}
	}

	// replicas
	if starttime == "" {
		msg := "Job not started"
		return inProgress(msg), nil
	}
	msg := fmt.Sprintf("Job in progress. success:%d, active: %d, failed: %d", succeeded, active, podFailed)
	return inProgress(msg), nil
}

// serviceConditions return standardized Conditions for Service
func serviceConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	specType := GetStringField(obj, ".spec.type", "ClusterIP")
	specClusterIP := GetStringField(obj, ".spec.clusterIP", "")

	if specType == "LoadBalancer" {
		if specClusterIP == "" {
			msg := "ClusterIP not set. Service type: LoadBalancer"
			return inProgress(msg), nil
		}
	}

	return ready("service ready"), nil
}

func crdConditions(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}

	for _, c := range objc.Status.Conditions {
		if c.Type == "NamesAccepted" && c.Status == metav1.ConditionFalse {
			return failed(c.Message), nil
		}
		if c.Type == "Established" {
			if c.Status == metav1.ConditionFalse && c.Reason != "Installing" {
				return failed(c.Message), nil
			}
			if c.Status == metav1.ConditionTrue {
				return ready("CRD established"), nil
			}
		}
	}
	return inProgress("installing"), nil
}
