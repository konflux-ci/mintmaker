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

package utils

import (
	"fmt"
	"net/url"
	"strings"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
)

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

func GetGitPlatform(giturl string) (string, error) {
	allowedGitPlatforms := []string{"github", "gitlab"}
	host, err := GetGitHost(giturl)
	if err != nil {
		return "", err
	}

	var gitPlatform string
	for _, platform := range allowedGitPlatforms {
		if strings.Contains(host, platform) {
			gitPlatform = platform
			break
		}
	}
	if gitPlatform == "" {
		return "", fmt.Errorf("unsupported git platform for repository %s", giturl)
	}
	return gitPlatform, nil
}

func GetGitHost(giturl string) (string, error) {
	// Handle SSH URLs (user@host:path)
	if strings.Contains(giturl, "@") {
		parts := strings.SplitN(giturl, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid SSH URL format: %s", giturl)
		}
		hostPart := strings.SplitN(parts[0], "@", 2)
		if len(hostPart) != 2 {
			return "", fmt.Errorf("invalid SSH URL format: %s", giturl)
		}
		return hostPart[1], nil
	}

	u, err := url.Parse(giturl)
	if err != nil {
		return "", err
	}
	host := u.Hostname()

	return host, nil
}

func GetGitPath(giturl string) (string, error) {
	giturl = strings.TrimSuffix(strings.TrimSuffix(giturl, "/"), ".git")
	// Handle SSH URLs (user@host:path)
	if strings.Contains(giturl, "@") {
		parts := strings.SplitN(giturl, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid SSH URL format: %s", giturl)
		}
		return strings.TrimPrefix(parts[1], "/"), nil
	}

	u, err := url.Parse(giturl)
	if err != nil {
		return "", err
	}
	path := strings.TrimPrefix(u.Path, "/")
	return path, nil
}

func GetVersions(comp *appstudiov1alpha1.Component) []string {
	if comp.Spec.Source.GitSource != nil && comp.Spec.Source.GitSource.Revision != "" {
		return []string{comp.Spec.Source.GitSource.Revision}
	}

	versions := []string{}
	for _, v := range comp.Status.Versions {
		if v.OnboardingStatus == "succeeded" {
			versions = append(versions, v.Revision)
		}
	}

	return versions
}
