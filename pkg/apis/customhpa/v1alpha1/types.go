package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
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
	ObservedGeneration *int64                                           `json:"observedGeneration"`
	LastScaleTime      *metav1.Time                                     `json:"lastScaleTime"`
	CurrentReplicas    int32                                            `json:"currentReplicas"`
	DesiredReplicas    int32                                            `json:"desiredReplicas"`
	CurrentMetrics     []autoscalingv2.MetricStatus                     `json:"currentMetrics"`
	Conditions         []autoscalingv2.HorizontalPodAutoscalerCondition `json:"conditions"`
}

// CustomHPASpec is the spec for a CustomHPA resource
type CustomHPASpec struct {
	HorizontalPodAutoscalerName string                                         `json:"horizontalPodAutoscalerName"`
	MinReplicas                 *int32                                         `json:"minReplicas"`
	MinReplicasTraining         *int32                                         `json:"minReplicasTraining"`
	MaxReplicas                 int32                                          `json:"maxReplicas"`
	MaxReplicasTraining         int32                                          `json:"maxReplicasTraining"`
	ScaleTargetRef              autoscalingv2.CrossVersionObjectReference      `json:"scaleTargetRef"`
	Metrics                     []autoscalingv2.MetricSpec                     `json:"metrics"`
	Behavior                    *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CustomHPAList is a list of CustomHPA resources
type CustomHPAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CustomHPA `json:"items"`
}
