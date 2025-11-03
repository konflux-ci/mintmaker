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

	github "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
	gitlab "github.com/konflux-ci/mintmaker/internal/pkg/component/gitlab"
	utils "github.com/konflux-ci/mintmaker/internal/pkg/utils"
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
	GetBranch() (string, error)
	GetAPIEndpoint() string
	GetRenovateConfig(*corev1.Secret) (string, error)
	GetRPMActivationKey(client.Client, context.Context) (string, string, error)
	GetRPMSSLAuth(client.Client, context.Context) (string, string, error)
}

func NewGitComponent(comp *appstudiov1alpha1.Component, client client.Client, ctx context.Context) (GitComponent, error) {
	// First check if source url exists and is properly defined
	if comp.Spec.Source.GitSource == nil || comp.Spec.Source.GitSource.URL == "" {
		return nil, fmt.Errorf("component %s has no git source or empty URL defined", comp.Name)
	}
	platform, err := utils.GetGitPlatform(comp.Spec.Source.GitSource.URL)
	if err != nil {
		return nil, err
	}

	switch platform {
	case "github":
		c, err := github.NewComponent(comp, client, ctx)
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	case "gitlab":
		c, err := gitlab.NewComponent(comp, client, ctx)
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}
