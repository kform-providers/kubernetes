package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Reason string

const (
	ReasonReady        Reason = "Ready"
	ReasonTerminating  Reason = "Terminating"
	ReasonInProgress   Reason = "InProgress"
	ReasonNoStatusInfo Reason = "NoStatusInfo"
	ReasonUserManaged  Reason = "UserManaged"
	ReasonFailed       Reason = "Failed"
)

// A ConditionType represents a condition type for a given KRM resource
type ConditionType string

// Condition Types.
const (
	// ConditionTypeReady represents the resource ready condition
	ConditionTypeReady ConditionType = "Ready"
)

// Result contains the results of a call to compute the status of
// a resource.
type Result struct {
	Status metav1.ConditionStatus
	// Reason
	Reason Reason
	// Message
	Message string
}

func Compute(u *unstructured.Unstructured) (*Result, error) {
	res, err := checkGenericProperties(u)
	if err != nil {
		return nil, err
	}
	// If res is not nil, the status was determined by the generic rules and we can conclude
	// the status
	if res != nil {
		return res, nil
	}

	fn := GetLegacyConditionsFn(u)
	if fn != nil {
		return fn(u)
	}

	return noStatusInfo(), err
}

func ready(msg string) *Result {
	return &Result{
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReady,
		Message: msg,
	}
}

func noStatusInfo() *Result {
	return &Result{
		Status: metav1.ConditionTrue,
		Reason: ReasonNoStatusInfo,
	}
}

func userManaged() *Result {
	return &Result{
		Status: metav1.ConditionTrue,
		Reason: ReasonUserManaged,
	}
}

func inProgress(msg string) *Result {
	return &Result{
		Status:  metav1.ConditionFalse,
		Reason:  ReasonInProgress,
		Message: msg,
	}
}

func terminating() *Result {
	return &Result{
		Status: metav1.ConditionFalse,
		Reason: ReasonTerminating,
	}
}

func failed(msg string) *Result {
	return &Result{
		Status:  metav1.ConditionFalse,
		Reason:  ReasonFailed,
		Message: msg,
	}
}
