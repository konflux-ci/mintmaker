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

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/component"
)

func ignoreMissingCRD(err error) error {
	if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// Get only components that match a given namespace/componentname
func getFilteredComponents(ctx context.Context, namespaces []mmv1alpha1.NamespaceSpec, apiClient client.Client) ([]konfluxv1alpha1.Component, error) {
	components := []konfluxv1alpha1.Component{}

	// Iterate namespaces and create query filtered by namespace
	for _, namespace := range namespaces {
		konfluxList := &konfluxv1alpha1.ComponentList{}
		if err := apiClient.List(ctx, konfluxList, client.InNamespace(namespace.Namespace)); err != nil {
			if err := ignoreMissingCRD(err); err != nil {
				return nil, err
			}
		}
		legacyList := &appstudiov1alpha1.ComponentList{}
		if err := apiClient.List(ctx, legacyList, client.InNamespace(namespace.Namespace)); err != nil {
			if err := ignoreMissingCRD(err); err != nil {
				return nil, err
			}
		}
		merged := mergeKonfluxAndLegacyComponents(konfluxList.Items, legacyList.Items)

		// No applications specified -> add all Namespace components, start processing next namespace
		if len(namespace.Applications) == 0 {
			components = append(components, merged...)
			continue
		}

		// New component model does not include application name.
		// Only component names are used for filtering.
		for _, application := range namespace.Applications {
			// No components specified -> add all namespace components
			if len(application.Components) == 0 {
				components = append(components, merged...)
				break
			}
			// Components specified -> add components with matching names
			for _, filterComponent := range application.Components {
				for _, comp := range merged {
					if filterComponent == mmv1alpha1.Component(comp.Name) {
						components = append(components, comp)
						break
					}
				}
			}
		}
	}

	return components, nil
}

func mergeKonfluxAndLegacyComponents(konflux []konfluxv1alpha1.Component, legacy []appstudiov1alpha1.Component) []konfluxv1alpha1.Component {
	seen := make(map[string]struct{}, len(konflux)+len(legacy))
	out := make([]konfluxv1alpha1.Component, 0, len(konflux)+len(legacy))

	for i := range konflux {
		c := konflux[i]
		key := c.Namespace + "/" + c.Name
		seen[key] = struct{}{}
		out = append(out, c)
	}

	for i := range legacy {
		key := legacy[i].Namespace + "/" + legacy[i].Name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, *component.ToKonfluxComponent(&legacy[i]))
	}

	return out
}
