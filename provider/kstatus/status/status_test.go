package status

import (
	"testing"

	"github.com/kform-providers/kubernetes/provider/kstatus/status/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var deploymentManifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   conditions:
    - type: Progressing
      status: "True"
      reason: NewReplicaSetAvailable
    - type: Available
      status: "True"
`

var configMapManifest = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: edge01
  namespace: default
  resourceVersion: "803002"
  uid: d777644a-8779-44c9-a6f7-c2e1b0499f5f
data:
  attachmentType: vlan
  clusterName: edge10
  count: "5"
  revision: v1
`

var targetReadyManifest = `
apiVersion: inv.sdcio.dev/v1alpha1
kind: Target
metadata:
  annotations:
    inv.sdcio.dev/discovery-rule: dr-static
  creationTimestamp: "2024-03-29T00:58:41Z"
  finalizers:
  - targetdatastore.inv.sdcio.dev/finalizer
  generation: 1
  labels:
    inv.sdcio.dev/discovery-rule: dr-static
    sdcio.dev/region: us-east
  name: dev1
  namespace: default
  ownerReferences:
  - apiVersion: inv.sdcio.dev/v1alpha1
    controller: true
    kind: DiscoveryRule
    name: dr-static
    uid: 8408e604-20b7-45c3-af7b-e0452aa41f44
  resourceVersion: "695865"
  uid: 23a60558-90d0-4e5e-811d-c9c04cb80e58
spec:
  address: 172.18.0.4:57400
  connectionProfile: gnmi-skipverify
  credentials: srl.nokia.sdcio.dev
  provider: srl.nokia.sdcio.dev
  syncProfile: gnmi-onchange
status:
  conditions:
  - lastTransitionTime: "2024-03-29T00:58:41Z"
    message: ""
    reason: Ready
    status: "True"
    type: DiscoveryReady
  - lastTransitionTime: "2024-03-29T16:10:31Z"
    message: ""
    reason: Ready
    status: "True"
    type: ConfigReady
  - lastTransitionTime: "2024-03-29T16:10:31Z"
    message: ""
    reason: Ready
    status: "True"
    type: Ready
  - lastTransitionTime: "2024-03-29T16:10:29Z"
    message: ""
    reason: Ready
    status: "True"
    type: DatastoreReady
  discoveryInfo:
    hostname: dev1
    lastSeen: "2024-04-03T13:14:10Z"
    protocol: static
    provider: srl.nokia.sdcio.dev
    version: 23.10.1
  usedReferences:
    connectionProfileResourceVersion: "680"
    secretResourceVersion: "682"
    syncProfileResourceVersion: "683"
`

var targetInProgressManifest = `
apiVersion: inv.sdcio.dev/v1alpha1
kind: Target
metadata:
  annotations:
    inv.sdcio.dev/discovery-rule: dr-static
  creationTimestamp: "2024-03-29T00:58:41Z"
  finalizers:
  - targetdatastore.inv.sdcio.dev/finalizer
  generation: 1
  labels:
    inv.sdcio.dev/discovery-rule: dr-static
    sdcio.dev/region: us-east
  name: dev1
  namespace: default
  ownerReferences:
  - apiVersion: inv.sdcio.dev/v1alpha1
    controller: true
    kind: DiscoveryRule
    name: dr-static
    uid: 8408e604-20b7-45c3-af7b-e0452aa41f44
  resourceVersion: "695865"
  uid: 23a60558-90d0-4e5e-811d-c9c04cb80e58
spec:
  address: 172.18.0.4:57400
  connectionProfile: gnmi-skipverify
  credentials: srl.nokia.sdcio.dev
  provider: srl.nokia.sdcio.dev
  syncProfile: gnmi-onchange
status:
  conditions:
  - lastTransitionTime: "2024-03-29T00:58:41Z"
    message: ""
    reason: Ready
    status: "True"
    type: DiscoveryReady
  - lastTransitionTime: "2024-03-29T16:10:31Z"
    message: ""
    reason: Ready
    status: "True"
    type: ConfigReady
  - lastTransitionTime: "2024-03-29T16:10:31Z"
    message: ""
    reason: Ongoing
    status: "False"
    type: Ready
  - lastTransitionTime: "2024-03-29T16:10:29Z"
    message: ""
    reason: Ready
    status: "True"
    type: DatastoreReady
  discoveryInfo:
    hostname: dev1
    lastSeen: "2024-04-03T13:14:10Z"
    protocol: static
    provider: srl.nokia.sdcio.dev
    version: 23.10.1
  usedReferences:
    connectionProfileResourceVersion: "680"
    secretResourceVersion: "682"
    syncProfileResourceVersion: "683"
`

func TestCompute(t *testing.T) {
	cases := map[string]struct {
		yaml   string
		result *Result
	}{
		"Deployment": {
			yaml: deploymentManifest,
			result: &Result{
				Status:  metav1.ConditionTrue,
				Reason:  ReasonReady,
				Message: "Deployment is available. Replicas: 1",
			},
		},
		"ConfigMap": {
			yaml: configMapManifest,
			result: &Result{
				Status:  metav1.ConditionTrue,
				Reason:  ReasonReady,
				Message: "ready",
			},
		},
		"TargetReady": {
			yaml: targetReadyManifest,
			result: &Result{
				Status: metav1.ConditionTrue,
				Reason: ReasonReady,
			},
		},
		"TargetInProgress": {
			yaml: targetInProgressManifest,
			result: &Result{
				Status: metav1.ConditionFalse,
				Reason: ReasonInProgress,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			u := testutil.YamlToUnstructured(t, tc.yaml)

			res, err := Compute(u)
			assert.NoError(t, err)
			assert.Equal(t, *res, *tc.result)
		})
	}
}
