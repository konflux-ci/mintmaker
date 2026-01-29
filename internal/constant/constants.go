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

package constant

const (
	// The namespace name where mintmaker is running
	MintMakerNamespaceName = "mintmaker"
	// Mintmaker will add processed annotation when the dependencyupdatecheck is processed by controller
	MintMakerProcessedAnnotationName = "mintmaker.appstudio.redhat.com/processed"
	// Mintmaker can be disabled by disabled annotation in component
	MintMakerDisabledAnnotationName = "mintmaker.appstudio.redhat.com/disabled"
	// Label for the Kite token secret, used to find the secret in the namespace
	KiteTokenSecretLabel = "mintmaker.appstudio.redhat.com/kite-token"

	RenovateImageEnvName    = "RENOVATE_IMAGE"
	DefaultRenovateImageURL = "quay.io/konflux-ci/mintmaker-renovate-image:latest"
)
