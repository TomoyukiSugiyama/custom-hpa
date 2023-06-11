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
	AvailableReplicas int32 `json:"availableReplicas"`
}

// CustomHPASpec is the spec for a CustomHPA resource
type CustomHPASpec struct {
	DeploymentName string `json:"deploymentName"`
	Replicas       *int32 `json:"replicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomHPAList is a list of CustomHPA resources
type CustomHPAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CustomHPA `json:"items"`
}
