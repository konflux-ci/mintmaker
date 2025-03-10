// Copyright 2024 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
// +kubebuilder:validation:MaxLength=63
type Component string

type ApplicationSpec struct {
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// Specifies the name of the application for which to run Mintmaker.
	// Required.
	// +required
	Application string `json:"application"`

	// Specifies the list of components of an application for which to run MintMaker.
	// If omitted, MintMaker will run for all application's components.
	// +optional
	Components []Component `json:"components,omitempty"`
}

type WorkspaceSpec struct {
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	//Specifies the name of the workspace for which to run Mintmaker.
	// Required.
	// +required
	Workspace string `json:"workspace"`

	// Specifies the list of applications in a workspace for which to run MintMaker.
	// If omitted, MintMaker will run for all workspace's applications.
	// +optional
	Applications []ApplicationSpec `json:"applications,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DependencyUpdateCheckSpec defines the desired state of DependencyUpdateCheck
type DependencyUpdateCheckSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Specifies the list of workspaces for which to run MintMaker.
	// If omitted, MintMaker will run for all workspaces.
	// +optional
	Workspaces []WorkspaceSpec `json:"workspaces,omitempty"`
}

// DependencyUpdateCheckStatus defines the observed state of DependencyUpdateCheck
type DependencyUpdateCheckStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DependencyUpdateCheck is the Schema for the dependencyupdatechecks API
type DependencyUpdateCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DependencyUpdateCheckSpec   `json:"spec,omitempty"`
	Status DependencyUpdateCheckStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DependencyUpdateCheckList contains a list of DependencyUpdateCheck
type DependencyUpdateCheckList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DependencyUpdateCheck `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DependencyUpdateCheck{}, &DependencyUpdateCheckList{})
}
