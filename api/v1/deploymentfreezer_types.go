/*
Copyright 2025.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeploymentFreezerSpec defines the desired state of DeploymentFreezer
type DeploymentFreezerSpec struct {
	DeploymentName      string `json:"deploymentName"`
	FreezeDurationInSec int32  `json:"freezeDurationInSec"`
}

// DeploymentFreezerStatus defines the observed state of DeploymentFreezer.
type DeploymentFreezerStatus struct {
	PrevReplicaCount *int32       `json:"prevReplicaCount"`
	CompletionTime   *metav1.Time `json:"completionTime,omitempty"`
	FreezeEndTime    *metav1.Time `json:"freezeStartTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DeploymentFreezer is the Schema for the deploymentfreezers API
type DeploymentFreezer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of DeploymentFreezer
	// +required
	Spec DeploymentFreezerSpec `json:"spec"`

	// status defines the observed state of DeploymentFreezer
	// +optional
	Status DeploymentFreezerStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// DeploymentFreezerList contains a list of DeploymentFreezer
type DeploymentFreezerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentFreezer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentFreezer{}, &DeploymentFreezerList{})
}
