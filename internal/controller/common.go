/*
Copyright 2024 Red Hat, Inc.

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

package controller

import (
	"context"
	"fmt"
	"strings"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	gp "github.com/konflux-ci/mintmaker/pkg/git/gitprovider"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Annotation that specifies git provider id for self hosted SCM instances, e.g. github or gitlab.
	GitProviderAnnotationName = "git-provider"
)

func getGitProvider(component appstudiov1alpha1.Component) (string, error) {
	allowedGitProviders := map[string]bool{"github": true, "gitlab": true}
	gitProvider := ""

	if component.Spec.Source.GitSource == nil {
		err := fmt.Errorf("git source URL is not set for %s Component in %s namespace", component.Name, component.Namespace)
		return "", err
	}

	// If possible, parse git provider from the repository URL
	sourceUrl, err := gp.ParseGitURL(component.Spec.Source.GitSource.URL)
	if err == nil {
		gitProvider = strings.Split(sourceUrl.Hostname(), ".")[0]
	}

	// Self-hosted git provider, check for git-provider annotation on the component
	if !allowedGitProviders[gitProvider] {
		gitProviderAnnotationValue := component.GetAnnotations()[GitProviderAnnotationName]
		if gitProviderAnnotationValue != "" {
			if allowedGitProviders[gitProviderAnnotationValue] {
				gitProvider = gitProviderAnnotationValue
			} else {
				err = fmt.Errorf("unsupported \"%s\" annotation value: %s", GitProviderAnnotationName, gitProviderAnnotationValue)
			}
		} else {
			err = fmt.Errorf("self-hosted git provider is not specified via \"%s\" annotation in the component", GitProviderAnnotationName)
		}
	}

	return gitProvider, err
}

// Get only components that match a given workspace/application/componentname
func getFilteredComponents(workspaces []mmv1alpha1.WorkspaceSpec, apiClient client.Client, ctx context.Context) ([]appstudiov1alpha1.Component, error) {
	components := []appstudiov1alpha1.Component{}
	err := error(nil)

	// Iterate workspaces and create query filtered by namespace ({workspace}-tenant)
	for _, workspace := range workspaces {
		workspaceComponentList := &appstudiov1alpha1.ComponentList{}
		listOps := &client.ListOptions{
			Namespace: workspace.Workspace + "-tenant",
		}
		if err := apiClient.List(ctx, workspaceComponentList, listOps); err != nil {
			return nil, err
		}
		// No applications specified -> add all Workspace components, start processing next workspace
		if len(workspace.Applications) == 0 {
			components = append(components, workspaceComponentList.Items...)
			continue
		}
		// Applications specified -> iterate and filter by application
		for _, application := range workspace.Applications {
			appMatchingComponents := []appstudiov1alpha1.Component{}
			for _, component := range workspaceComponentList.Items {
				if application.Application == component.Spec.Application {
					appMatchingComponents = append(appMatchingComponents, component)
				}
			}
			// No components specified for an application -> add all application components, start processing next application
			if len(application.Components) == 0 {
				components = append(components, appMatchingComponents...)
				continue
			}
			// Components specified -> add components with matching names
			for _, filterComponent := range application.Components {
				for _, component := range appMatchingComponents {
					if filterComponent == mmv1alpha1.Component(component.Spec.ComponentName) {
						components = append(components, component)
						break
					}
				}
			}
		}
	}

	return components, err
}
