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

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get only components that match a given namespace/component name
func getFilteredComponents(ctx context.Context, namespaces []mmv1alpha1.NamespaceSpec, apiClient client.Client) ([]appstudiov1alpha1.Component, error) {
	components := []appstudiov1alpha1.Component{}
	err := error(nil)

	// Iterate namespaces and create query filtered by namespace
	for _, namespace := range namespaces {
		namespaceComponentList := &appstudiov1alpha1.ComponentList{}
		listOps := &client.ListOptions{
			Namespace: namespace.Namespace,
		}
		if err := apiClient.List(ctx, namespaceComponentList, listOps); err != nil {
			return nil, err
		}
		// No components specified -> add all namespace components, start processing next namespace
		if len(namespace.Components) == 0 {
			components = append(components, namespaceComponentList.Items...)
			continue
		}
		// Components specified -> add components with matching names
		for _, filterComponent := range namespace.Components {
			for _, component := range namespaceComponentList.Items {
				if filterComponent == mmv1alpha1.Component(component.Name) {
					components = append(components, component)
					break
				}
			}
		}
	}

	return components, err
}
