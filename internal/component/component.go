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

package component

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	github "github.com/konflux-ci/mintmaker/internal/component/github"
	gitlab "github.com/konflux-ci/mintmaker/internal/component/gitlab"
	utils "github.com/konflux-ci/mintmaker/internal/utils"
)

type GitComponent interface {
	GetName() string
	GetNamespace() string
	GetApplication() string
	GetPlatform() string
	GetHost() string
	GetGitURL() string
	GetRepository() string
	GetToken() (string, error)
	GetBranches() []string
	GetAPIEndpoint() string
	GetRenovateConfig(*corev1.Secret, string) (string, error)
	GetRPMActivationKey(context.Context, client.Client) (string, string, error)
}

func NewGitComponent(ctx context.Context, comp *appstudiov1alpha1.Component, client client.Client) (GitComponent, error) {
	// First check if source url exists and is properly defined
	gitUrl, err := GetGitURL(comp)
	if err != nil {
		return nil, err
	}

	platform, err := utils.GetGitPlatform(gitUrl)
	if err != nil {
		return nil, err
	}

	switch platform {
	case "github":
		c, err := github.NewComponent(ctx, comp, client, gitUrl, GetVersions(comp))
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	case "gitlab":
		c, err := gitlab.NewComponent(ctx, comp, client, gitUrl, GetVersions(comp))
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// GetGitURL returns the git URL for the component
// It supports both the old and new component models
func GetGitURL(comp *appstudiov1alpha1.Component) (string, error) {
	if comp.Spec.Source.GitSource != nil && comp.Spec.Source.GitSource.URL != "" {
		return comp.Spec.Source.GitSource.URL, nil
	}

	if comp.Spec.Source.GitURL != "" {
		return comp.Spec.Source.GitURL, nil
	}

	return "", fmt.Errorf("component %s has no git source or empty URL defined", comp.Name)
}

func GetVersions(comp *appstudiov1alpha1.Component) []string {
	if comp.Spec.Source.GitSource != nil && comp.Spec.Source.GitSource.Revision != "" {
		return []string{comp.Spec.Source.GitSource.Revision}
	}

	versions := make([]string, 0, len(comp.Spec.Source.Versions))
	for _, v := range comp.Spec.Source.Versions {
		versions = append(versions, v.Revision)
	}

	return versions
}
