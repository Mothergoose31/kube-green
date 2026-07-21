/*
Copyright 2021.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SleepInfoExecutionOperation is the operation to perform on the SleepInfo target resources.
// +kubebuilder:validation:Enum=SLEEP;WAKE_UP
type SleepInfoExecutionOperation string

const (
	// ExecutionOpSleep triggers an on-demand sleep of the resources managed by the referenced SleepInfo.
	ExecutionOpSleep SleepInfoExecutionOperation = "SLEEP"
	// ExecutionOpWake triggers an on-demand wake up of the resources managed by the referenced SleepInfo.
	ExecutionOpWake SleepInfoExecutionOperation = "WAKE_UP"
)

// SleepInfoExecutionPhase describes the lifecycle of a SleepInfoExecution.
type SleepInfoExecutionPhase string

const (
	// ExecutionPhasePending means the execution has not been processed yet.
	ExecutionPhasePending SleepInfoExecutionPhase = "Pending"
	// ExecutionPhaseSucceeded means the operation was performed successfully.
	ExecutionPhaseSucceeded SleepInfoExecutionPhase = "Succeeded"
	// ExecutionPhaseFailed means the operation could not be performed.
	ExecutionPhaseFailed SleepInfoExecutionPhase = "Failed"
	// ExecutionPhaseSkipped means the operation was a no-op, e.g. a wake up
	// requested while the namespace resources were not asleep.
	ExecutionPhaseSkipped SleepInfoExecutionPhase = "Skipped"
)

// SleepInfoExecutionSpec defines the desired state of SleepInfoExecution
type SleepInfoExecutionSpec struct {
	// SleepInfoName is the name of the SleepInfo, in the same namespace of the
	// SleepInfoExecution, to run the operation for.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:MinLength=1
	SleepInfoName string `json:"sleepInfoName"`
	// Operation to perform on the resources managed by the referenced
	// SleepInfo. SLEEP or WAKE_UP are the possibilities.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Operation SleepInfoExecutionOperation `json:"operation"`
}

// SleepInfoExecutionStatus defines the observed state of SleepInfoExecution
type SleepInfoExecutionStatus struct {
	// Phase of the execution. Pending, Succeeded, Failed or Skipped are the
	// possibilities.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Phase"
	Phase SleepInfoExecutionPhase `json:"phase,omitempty"`
	// Information when the execution started to be processed.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Start Time"
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Information when the execution completed.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Completion Time"
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// Message is a human readable information about the execution outcome.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Message"
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=sleepinfoexecutions
// +kubebuilder:printcolumn:name="SleepInfo",type=string,JSONPath=`.spec.sleepInfoName`
// +kubebuilder:printcolumn:name="Operation",type=string,JSONPath=`.spec.operation`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +operator-sdk:csv:customresourcedefinitions:displayName="SleepInfoExecution"
// +genclient - this is required for auto generated docs

// SleepInfoExecution is the Schema for the sleepinfoexecutions API.
// It triggers an on-demand sleep or wake up of the resources managed by the
// referenced SleepInfo, regardless of the configured schedule. Once processed,
// the object is kept as a historical record of the execution.
type SleepInfoExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is immutable: a SleepInfoExecution is a one-shot request. Create a
	// new resource to trigger another operation.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
	Spec   SleepInfoExecutionSpec   `json:"spec"`
	Status SleepInfoExecutionStatus `json:"status,omitempty"`
}

// IsProcessed returns true when the execution reached a terminal phase.
func (s SleepInfoExecution) IsProcessed() bool {
	switch s.Status.Phase {
	case ExecutionPhaseSucceeded, ExecutionPhaseFailed, ExecutionPhaseSkipped:
		return true
	default:
		return false
	}
}

// +kubebuilder:object:root=true

// SleepInfoExecutionList contains a list of SleepInfoExecution
type SleepInfoExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SleepInfoExecution `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SleepInfoExecution{}, &SleepInfoExecutionList{})
}
