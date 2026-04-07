/*
Copyright 2024.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SleepInfoExecutionOperation is a sleep or wake operation.
// +kubebuilder:validation:Enum=SLEEP;WAKE
type SleepInfoExecutionOperation string

// SleepInfoExecutionPhase is lifecycle phase of execution.
// +kubebuilder:validation:Enum=Pending;Running;Failed;Succeeded
type SleepInfoExecutionPhase string

const (
	ExecutionOpSleep SleepInfoExecutionOperation = "SLEEP"
	ExecutionOpWake  SleepInfoExecutionOperation = "WAKE"
)

const (
	PhasePending   SleepInfoExecutionPhase = "Pending"
	PhaseRunning   SleepInfoExecutionPhase = "Running"
	PhaseSucceeded SleepInfoExecutionPhase = "Succeeded"
	PhaseFailed    SleepInfoExecutionPhase = "Failed"
)

// SleepinfoExecutionSpec defines the desired state of SleepinfoExecution
type SleepinfoExecutionSpec struct {
	// SleepinfoRef nam of SleepInfo in name space to run against
	// +kubebuilder:validation:MinLength=1
	SleepInfoRef string `json:"SleepInfoRef"`
	// Operation to perform
	Operation SleepInfoExecutionOperation `json:"operation"`
}

// SleepinfoExecutionStatus defines the observed state of SleepinfoExecution.
type SleepinfoExecutionStatus struct {
	// Life cycle phase
	// +optional
	Phase SleepInfoExecutionPhase `json:"phase,omitempty"`
	// metadata.generation
	// +optionnal
	ObservedGeneration int64 `json:"observedgeneration,omitempty"`
	// details of outcome
	// +optionnal
	Message string `json:"message,omitempty"`
	// represent latest observation of the execution
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// SleepInfoExecutionSpec defines the desired state of SleepInfoExecution.
type SleepInfoExecutionSpec struct {
	// SleepInfoRef is the name of the SleepInfo in the same namespace to run against.
	// +kubebuilder:validation:MinLength=1
	SleepInfoRef string `json:"SleepInfoRef"`
	// Operation is the one-shot operation to perform.
	Operation SleepInfoExecutionOperation `json:"operation"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=sleepinfoexecutions
// +kubebuilder:printcolumn:name="SleepInfo",type="string",JSONPath=`.spec.SleepInfoRef`
// +kubebuilder:printcolumn:name="Operation",type="string",JSONPath=`.spec.operation`
// +kubebuilder:printcloumn:name="Phase",type="string",JSONPath=`status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.createTimestamp`

type SleepinfoExecution struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SleepinfoExecution
	// +required
	Spec SleepinfoExecutionSpec `json:"spec"`

	// status defines the observed state of SleepinfoExecution
	// +optional
	Status SleepinfoExecutionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SleepinfoExecutionList contains a list of SleepinfoExecution
type SleepinfoExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SleepinfoExecution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SleepinfoExecution{}, &SleepinfoExecutionList{})
}
