// Copyright 2026 Red Hat, Inc.
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

package component

import (
	"maps"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

// ToKonfluxComponent maps a legacy appstudio Component into a konflux Component
// for MintMaker factory use only during the migration to the new domain.
// Remove when appstudio Components are decommissioned.
func ToKonfluxComponent(src *appstudiov1alpha1.Component) *konfluxv1alpha1.Component {
	dst := &konfluxv1alpha1.Component{}
	dst.Name = src.Name
	dst.Namespace = src.Namespace

	if src.Annotations != nil {
		dst.Annotations = maps.Clone(src.Annotations)
	}

	// Convert application name from old model to annotation.
	if src.Spec.Application != "" {
		if dst.Annotations == nil {
			dst.Annotations = map[string]string{}
		}
		dst.Annotations[mmconst.MintMakerConvertedApplicationAnnotation] = src.Spec.Application
	}

	// Flatten old GitSource model to new Source model (url + versions only).
	if src.Spec.Source.GitSource != nil && src.Spec.Source.GitSource.URL != "" {
		dst.Spec.Source.GitURL = src.Spec.Source.GitSource.URL
		if rev := src.Spec.Source.GitSource.Revision; rev != "" {
			dst.Spec.Source.Versions = []konfluxv1alpha1.ComponentVersion{{
				Name:     rev, // Name is required on konflux ComponentVersion
				Revision: rev,
			}}
		}
		if dst.Annotations == nil {
			dst.Annotations = map[string]string{}
		}
		dst.Annotations[mmconst.MintMakerOldCRDAnnotation] = "true"
		return dst
	}

	dst.Spec.Source.GitURL = src.Spec.Source.GitURL
	if len(src.Spec.Source.Versions) > 0 {
		dst.Spec.Source.Versions = make([]konfluxv1alpha1.ComponentVersion, len(src.Spec.Source.Versions))
		for i, v := range src.Spec.Source.Versions {
			name := v.Name
			if name == "" {
				name = v.Revision
			}
			dst.Spec.Source.Versions[i] = konfluxv1alpha1.ComponentVersion{
				Name:     name,
				Revision: v.Revision,
			}
		}
	}

	return dst
}
