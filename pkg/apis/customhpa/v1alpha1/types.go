package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// CustomHPA is a specification for a CustomHPA resource
type CustomHPA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status CustomHPAStatus `json:"status"`
	Spec   CustomHPASpec   `json:"spec"`
}

type CustomHPAStatus struct {
	CurrentReplicas int32 `json:"currentReplicas"`
	DesiredReplicas int32 `json:"desiredReplicas"`
}

// CustomHPASpec is the spec for a CustomHPA resource
type CustomHPASpec struct {
	MinReplicas         int32          `json:"minReplicas"`
	MinReplicasTraining int32          `json:"minReplicasTraining"`
	MaxReplicas         int32          `json:"maxReplicas"`
	MaxReplicasTraining int32          `json:"maxReplicasTraining"`
	ScaleTargetRef      ScaleTargetRef `json:"scaleTargetRef"`
}

type ScaleTargetRef struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomHPAList is a list of CustomHPA resources
type CustomHPAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CustomHPA `json:"items"`
}
