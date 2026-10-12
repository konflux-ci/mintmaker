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
	MintMakerProcessedAnnotationNameLegacy = "mintmaker.appstudio.redhat.com/processed"
	MintMakerProcessedAnnotationName       = "mintmaker.konflux-ci.dev/processed"

	// Mintmaker can be disabled by disabled annotation in component
	MintMakerDisabledAnnotationNameLegacy = "mintmaker.appstudio.redhat.com/disabled"
	MintMakerDisabledAnnotationName       = "mintmaker.konflux-ci.dev/disabled"

	MintMakerApplicationLabelLegacy = "mintmaker.appstudio.redhat.com/application"
	MintMakerApplicationLabel       = "mintmaker.konflux-ci.dev/application"

	MintMakerComponentNameLabelLegacy = "mintmaker.appstudio.redhat.com/component"
	MintMakerComponentNameLabel       = "mintmaker.konflux-ci.dev/component"

	MintMakerComponentNamespaceLabelLegacy = "mintmaker.appstudio.redhat.com/namespace"
	MintMakerComponentNamespaceLabel       = "mintmaker.konflux-ci.dev/namespace"

	MintMakerGitPlatformLabelLegacy = "mintmaker.appstudio.redhat.com/git-platform"
	MintMakerGitPlatformLabel       = "mintmaker.konflux-ci.dev/git-platform"

	MintMakerGitHostLabelLegacy = "mintmaker.appstudio.redhat.com/git-host"
	MintMakerGitHostLabel       = "mintmaker.konflux-ci.dev/git-host"

	MintMakerRepositoryLabelLegacy = "mintmaker.appstudio.redhat.com/repository"
	MintMakerRepositoryLabel       = "mintmaker.konflux-ci.dev/repository"

	MintMakerBranchLabelLegacy = "mintmaker.appstudio.redhat.com/branch"
	MintMakerBranchLabel       = "mintmaker.konflux-ci.dev/branch"

	// Label for the Kite token secret, used to find the secret in the namespace
	KiteTokenSecretLabelLegacy = "mintmaker.appstudio.redhat.com/kite-token" //nolint:gosec // label name, not a credential
	KiteTokenSecretLabel       = "mintmaker.konflux-ci.dev/kite-token"       //nolint:gosec // label name, not a credential

	// Label storing a truncated SHA256 hash of host/repository@branch.
	// Used to find active PipelineRuns for a given repo+branch combination.
	// Using hash because k8s label value doesn't support `/` (in repo path)
	MintMakerRepoBranchHashLabelLegacy = "mintmaker.appstudio.redhat.com/repo-branch-hash"
	MintMakerRepoBranchHashLabel       = "mintmaker.konflux-ci.dev/repo-branch-hash"

	// Temporary: carries Spec.Application from a converted legacy Component.
	// In-memory only; remove when legacy Components are gone.
	MintMakerConvertedApplicationAnnotation = "mintmaker.konflux-ci.dev/converted-application"

	// Temporary: set on in-memory Components converted from legacy Component CRD.
	// Read by NewGitComponent to allow default-branch fallback. Never persisted to the cluster.
	// Remove when we stop supporting the old Component CRD.
	MintMakerOldCRDAnnotation = "mintmaker.konflux-ci.dev/old-crd"

	InternalSecretLabelLegacy = "appstudio.redhat.com/internal"
	InternalSecretLabel       = "konflux-ci.dev/internal"

	CredentialsLabelLegacy = "appstudio.redhat.com/credentials"
	CredentialsLabel       = "konflux-ci.dev/credentials"

	SCMHostLabelLegacy = "appstudio.redhat.com/scm.host"
	SCMHostLabel       = "konflux-ci.dev/scm.host"

	SCMRepositoryAnnotationLegacy = "appstudio.redhat.com/scm.repository"
	SCMRepositoryAnnotation       = "konflux-ci.dev/scm.repository"

	RenovateImageEnvName    = "RENOVATE_IMAGE"
	DefaultRenovateImageURL = "quay.io/konflux-ci/mintmaker-renovate-image:latest"

	LeakTKImageEnvName    = "LEAKTK_IMAGE"
	DefaultLeakTKImageURL = "quay.io/leaktk/leaktk:latest"
)
