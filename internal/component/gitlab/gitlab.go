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

package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"

	"github.com/konflux-ci/mintmaker/internal/component/base"
	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
	bslices "github.com/konflux-ci/mintmaker/internal/slices"
	"github.com/konflux-ci/mintmaker/internal/utils"
)

//TODO: doc about only supporting GitHub with the installed GitHub App

type Component struct {
	base.BaseComponent
	client client.Client
	ctx    context.Context
}

type Repository struct {
	BaseBranches []string
	Repository   string
}

func NewComponent(ctx context.Context, comp *konfluxv1alpha1.Component, client client.Client, giturl string, versions []string) (*Component, error) {
	// TODO: a helper to validate and parse the git url
	platform, err := utils.GetGitPlatform(giturl)
	if err != nil {
		return nil, err
	}
	host, err := utils.GetGitHost(giturl)
	if err != nil {
		return nil, err
	}
	repository, err := utils.GetGitPath(giturl)
	if err != nil {
		return nil, err
	}

	return &Component{
		BaseComponent: base.BaseComponent{
			Name:          comp.Name,
			Namespace:     comp.Namespace,
			Application:   comp.Annotations[mmconst.MintMakerConvertedApplicationAnnotation],
			Platform:      platform,
			Host:          host,
			GitURL:        giturl,
			Repository:    repository,
			Versions:      versions,
			OldCRDVersion: comp.Annotations[mmconst.MintMakerOldCRDAnnotation] == "true",
		},
		client: client,
		ctx:    ctx,
	}, nil
}

func (c *Component) GetBranches() ([]string, error) {
	if len(c.Versions) == 0 && c.OldCRDVersion {
		defaultBranch, err := c.getDefaultBranch()
		if err != nil {
			return []string{}, fmt.Errorf("component does not have a branch specified and failed to get default branch: %w", err)
		}
		return []string{defaultBranch}, nil
	}

	var branches []string
	client, err := c.getClient()
	if err != nil {
		return []string{}, fmt.Errorf("failed to get GitLab client: %w", err)
	}

	for _, version := range c.Versions {
		_, _, err := client.Branches.GetBranch(c.Repository, version, nil)
		if err == nil {
			branches = append(branches, version)
		}
	}

	if len(branches) == 0 {
		return []string{}, fmt.Errorf("no versions found or all versions are tags (not branches)")
	}

	return branches, nil
}

func (c *Component) lookupSecret() (*corev1.Secret, error) {
	labelSets := []map[string]string{
		{mmconst.CredentialsLabel: "scm", mmconst.SCMHostLabel: c.Host},
		{mmconst.CredentialsLabelLegacy: "scm", mmconst.SCMHostLabelLegacy: c.Host},
	}

	// find secrets that have the following labels:
	//	- "konflux-ci.dev/credentials": "scm"
	//	- "konflux-ci.dev/scm.host": <name of component host>
	// Migration note: Temporarily fetch secrets for both new and legacy labels.
	var secrets []corev1.Secret
	for _, labels := range labelSets {
		secretList := &corev1.SecretList{}
		if err := c.client.List(c.ctx, secretList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
			return nil, fmt.Errorf("failed to list scm secrets in namespace %s: %w", c.Namespace, err)
		}
		// filtering to get BasicAuth secrets and data is not empty
		secrets = append(secrets, bslices.Filter(secretList.Items, func(secret corev1.Secret) bool {
			return secret.Type == corev1.SecretTypeBasicAuth && len(secret.Data) > 0
		})...)
	}

	if len(secrets) == 0 {
		return nil, fmt.Errorf("no secrets available for git host %s", c.Host)
	}

	// secrets only match with component's host
	var hostOnlySecrets []corev1.Secret
	// map of secret index and its best path intersections count, i.e. the count of path parts matched,
	var potentialMatches = make(map[int]int, len(secrets))

	for index, secret := range secrets {
		repositoryAnnotation := utils.GetAnnotation(&secret, mmconst.SCMRepositoryAnnotation, mmconst.SCMRepositoryAnnotationLegacy)
		if repositoryAnnotation == "" {
			hostOnlySecrets = append(hostOnlySecrets, secret)
			continue
		}

		secretRepositories := strings.Split(repositoryAnnotation, ",")
		// trim possible prefix or suffix "/"
		for i, repository := range secretRepositories {
			secretRepositories[i] = strings.TrimPrefix(strings.TrimSuffix(repository, "/"), "/")
		}

		// this secret matches exactly the component's repository name
		if slices.Contains(secretRepositories, c.Repository) {
			return &secret, nil
		}

		// no direct match, check for wildcard match, i.e. org/repo/* matches org/repo/foo, org/repo/bar, etc.
		componentRepoParts := strings.Split(c.Repository, "/")

		// find wildcard repositories
		wildcardRepos := bslices.Filter(secretRepositories, func(s string) bool { return strings.HasSuffix(s, "*") })

		for _, repo := range wildcardRepos {
			i := bslices.Intersection(componentRepoParts, strings.Split(strings.TrimSuffix(repo, "*"), "/"))
			if i > 0 && potentialMatches[index] < i {
				// add whole secret index to potential matches
				potentialMatches[index] = i
			}
		}
	}

	if len(potentialMatches) == 0 {
		if len(hostOnlySecrets) == 0 {
			// no potential matches, no host matches, nothing to return
			return nil, fmt.Errorf("no secrets available for component")
		}
		// no potential matches, but we have host match secrets, return the first one
		return &hostOnlySecrets[0], nil
	}

	// some potential matches exist, find the best one
	var bestIndex, bestCount int
	for i, count := range potentialMatches {
		if count > bestCount {
			bestCount = count
			bestIndex = i
		}
	}
	return &secrets[bestIndex], nil
}

func (c *Component) GetToken() (string, error) {

	secret, err := c.lookupSecret()
	if err != nil {
		return "", err
	}
	return string(secret.Data[corev1.BasicAuthPasswordKey]), nil
}

func (c *Component) GetAPIEndpoint() string {
	return fmt.Sprintf("https://%s/api/v4/", c.Host)
}

func (c *Component) getDefaultBranch() (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab client: %w", err)
	}

	project, _, err := client.Projects.GetProject(c.Repository, nil)
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", fmt.Errorf("project info is empty in GitLab API response")
	}

	return project.DefaultBranch, nil
}

func (c *Component) GetRenovateConfig(registrySecret *corev1.Secret, currentBranch string) (string, error) {
	baseConfig, err := c.GetRenovateBaseConfig(c.ctx, c.client)
	if err != nil {
		return "", err
	}

	// Add component-specific hostRules if registrySecret is provided
	if registrySecret != nil {
		hostRules, err := c.GetHostRules(c.ctx, registrySecret)
		if err == nil && len(hostRules) > 0 {
			baseConfig["hostRules"] = hostRules
		}
	}

	baseConfig["platform"] = c.Platform
	baseConfig["endpoint"] = c.GetAPIEndpoint()
	// We don't need to set a username or gitAuthor for gitlab, since this is tight to a token
	baseConfig["username"] = ""
	baseConfig["gitAuthor"] = ""

	// TODO: perhaps in the future let's validate all these values

	repo := map[string]interface{}{
		"baseBranchPatterns": []string{currentBranch},
		"repository":         c.Repository,
	}
	baseConfig["repositories"] = []interface{}{repo}

	updatedConfig, err := json.MarshalIndent(baseConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling updated Renovate config: %v", err)
	}

	return string(updatedConfig), nil
}

func (c *Component) getClient() (*gitlab.Client, error) {
	token, err := c.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get GitLab token: %w", err)
	}
	u, err := url.Parse(c.GitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git url: %w", err)
	}
	baseUrl := u.Scheme + "://" + c.Host
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseUrl))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return client, nil
}
