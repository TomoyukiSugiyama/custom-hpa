/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

// CustomHPASpecApplyConfiguration represents an declarative configuration of the CustomHPASpec type for use
// with apply.
type CustomHPASpecApplyConfiguration struct {
	DeploymentName *string `json:"deploymentName,omitempty"`
	Replicas       *int32  `json:"replicas,omitempty"`
}

// CustomHPASpecApplyConfiguration constructs an declarative configuration of the CustomHPASpec type for use with
// apply.
func CustomHPASpec() *CustomHPASpecApplyConfiguration {
	return &CustomHPASpecApplyConfiguration{}
}

// WithDeploymentName sets the DeploymentName field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the DeploymentName field is set to the value of the last call.
func (b *CustomHPASpecApplyConfiguration) WithDeploymentName(value string) *CustomHPASpecApplyConfiguration {
	b.DeploymentName = &value
	return b
}

// WithReplicas sets the Replicas field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Replicas field is set to the value of the last call.
func (b *CustomHPASpecApplyConfiguration) WithReplicas(value int32) *CustomHPASpecApplyConfiguration {
	b.Replicas = &value
	return b
}
