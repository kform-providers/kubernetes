package status

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func checkGenericProperties(u *unstructured.Unstructured) (*Result, error) {
	obj := u.UnstructuredContent()

	// Check if the resource is scheduled for deletion
	deletionTimestamp, found, err := unstructured.NestedString(obj, "metadata", "deletionTimestamp")
	if err != nil {
		return nil, fmt.Errorf("cannot lookup metadata.deletionTimestamp from resource: %w", err)
	}
	if found && deletionTimestamp != "" {
		return terminating(), nil
	}
	res, err := checkGeneration(u)
	if res != nil || err != nil {
		return res, err
	}
	// Check if the resource has any of the standard conditions. If so, we just use them
	// and no need to look at anything else.
	objc, err := GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}
	for _, cond := range objc.Status.Conditions {
		if cond.Type == string(ConditionTypeReady) {
			if cond.Status == metav1.ConditionTrue {
				return ready(cond.Message), nil
			} else {
				return inProgress(cond.Message), nil
			}
		}
	}
	return nil, nil
}

func checkGeneration(u *unstructured.Unstructured) (*Result, error) {
	// ensure that the meta generation is observed
	generation, found, err := unstructured.NestedInt64(u.Object, "metadata", "generation")
	if err != nil {
		return nil, fmt.Errorf("cannot lookup metadata.generation from resource: %w", err)
	}
	if !found {
		return nil, nil
	}
	observedGeneration, found, err := unstructured.NestedInt64(u.Object, "status", "observedGeneration")
	if err != nil {
		return nil, fmt.Errorf("cannot lookup status.observedGeneration from resource: %w", err)
	}
	if found {
		if observedGeneration != generation {
			msg := fmt.Sprintf("%s generation is %d, but latest observed generation is %d", u.GetKind(), generation, observedGeneration)
			return inProgress(msg), nil
		}
	}
	return nil, nil
}
