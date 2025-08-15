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

// Component represents a Component name within a Konflux Application.
// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
// +kubebuilder:validation:MaxLength=63
type Component string

// ApplicationSpec scopes MintMaker to specific Components within a single Konflux Application.
type ApplicationSpec struct {
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// Specifies the name of the Konflux application for which to run Mintmaker.
	// For more details see <a href="https://github.com/konflux-ci/architecture/blob/main/architecture/core/hybrid-application-service.md">Konflux Application Service</a>.
	// Required.
	// +required
	Application string `json:"application"`

	// Specifies the list of components of an application for which to run MintMaker.
	// If omitted, MintMaker will run for all application's components.
	// +optional
	Components []Component `json:"components,omitempty"`
}

// NamespaceSpec scopes MintMaker to specific Applications within a Kubernetes namespace.
type NamespaceSpec struct {
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// Specifies the name of the Kubernetes namespace for which to run Mintmaker.
	// Required.
	// +required
	Namespace string `json:"namespace"`

	// Specifies the list of Konflux applications in a namespace for which to run MintMaker.
	// If omitted, MintMaker will run for all namespace's applications.
	// +optional
	Applications []ApplicationSpec `json:"applications,omitempty"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DependencyUpdateCheckSpec filters which Konflux Components will be scanned.
// If `namespaces` is empty, MintMaker scans all Components discoverable to the controller.
// If provided, MintMaker only scans Components that match the namespace/application/component filters.
type DependencyUpdateCheckSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Specifies the list of namespaces for which to run MintMaker.
	// If omitted, MintMaker will run for all namespaces.
	// +optional
	Namespaces []NamespaceSpec `json:"namespaces,omitempty"`
}

// DependencyUpdateCheckStatus defines the observed state of DependencyUpdateCheck
type DependencyUpdateCheckStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DependencyUpdateCheck is the root CRD that triggers mintmaker to Konflux Components for dependency updates.
// How the controller uses this CRD:
// - Only CRs created in the MintMaker namespace (see `MintMakerNamespaceName`) are processed.
// - When a CR is created, the controller discovers Konflux Components to scan:
//   - By default: all `appstudio.redhat.com/v1alpha1, Kind=Component` across the cluster
//   - Or: a filtered subset when `spec.namespaces` is provided
//   - For each unique repository+branch across those Components, the controller generates
//     one Tekton `PipelineRun` that scans the repository for dependency updates using Renovate.
//
// Annotations:
//   - `mintmaker.appstudio.redhat.com/processed`: set by the controller when the
//     DependencyUpdateCheck is processed, to avoid reprocessing the same CR.
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
