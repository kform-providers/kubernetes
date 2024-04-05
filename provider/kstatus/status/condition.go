package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ObjWithConditions Represent meta object with status.condition array
type ObjWithConditions struct {
	// Status as expected to be present in most compliant kubernetes resources
	Status ConditionedStatus `json:"status" yaml:"status" protobuf:"bytes,1,rep,name=status"`
}

// A ConditionedStatus reflects the observed status of a resource. Only
// one condition of each type may exist.
type ConditionedStatus struct {
	// Conditions of the resource.
	// +optional
	Conditions []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty" protobuf:"bytes,1,rep,name=conditions"`
}

type Condition struct {
	metav1.Condition `json:",inline" yaml:",inline" protobuf:"bytes,1,opt,name=condition"`
}

// GetObjectWithConditions return typed object
func GetObjectWithConditions(in map[string]interface{}) (*ObjWithConditions, error) {
	var out = new(ObjWithConditions)
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hasConditionWithStatus(conditions []Condition, conditionType string, status metav1.ConditionStatus) bool {
	_, found := getConditionWithStatus(conditions, conditionType, status)
	return found
}

func getConditionWithStatus(conditions []Condition, conditionType string, status metav1.ConditionStatus) (Condition, bool) {
	for _, c := range conditions {
		if c.Type == conditionType && c.Status == status {
			return c, true
		}
	}
	return Condition{}, false
}
