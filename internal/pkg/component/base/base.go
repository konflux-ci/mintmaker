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

package base

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	bslices "github.com/konflux-ci/mintmaker/internal/pkg/slices"
)

var (
	renovateBaseConfig      map[string]interface{}
	renovateBaseConfigMutex sync.RWMutex
)

type BaseComponent struct {
	Name        string
	Namespace   string
	Application string
	Platform    string
	Host        string
	GitURL      string
	// Path of repository, without hostname
	// Temporary field to make the implementation easy, it's part of GitURL, so they're duplicated
	Repository string
	Branch     string
}

func (c *BaseComponent) GetName() string {
	return c.Name
}

func (c *BaseComponent) GetNamespace() string {
	return c.Namespace
}

func (c *BaseComponent) GetApplication() string {
	return c.Application
}

func (c *BaseComponent) GetPlatform() string {
	return c.Platform
}

func (c *BaseComponent) GetHost() string {
	return c.Host
}

func (c *BaseComponent) GetGitURL() string {
	return c.GitURL
}

func (c *BaseComponent) GetRepository() string {
	return c.Repository
}

type HostRule map[string]string

func (c *BaseComponent) GetHostRules(ctx context.Context, registrySecret *corev1.Secret) ([]HostRule, error) {
	log := logger.FromContext(ctx)

	if _, exists := registrySecret.Data[corev1.DockerConfigJsonKey]; !exists {
		return nil, errors.New(fmt.Sprintf("Secret %s is missing the %s key", registrySecret.Name, corev1.DockerConfigJsonKey))
	}

	var hostRules []HostRule
	var secrets map[string]map[string]HostRule

	err := json.Unmarshal(registrySecret.Data[corev1.DockerConfigJsonKey], &secrets)

	if err != nil {
		log.Info(fmt.Sprintf("Cannot unmarshal registry secret: %s", err))
		return nil, err
	}

	for registry, credentials := range secrets["auths"] {
		hostRule := HostRule{}
		hostRule["matchHost"] = registry

		if _, ok := credentials["auth"]; ok {
			auth_plain, err := base64.StdEncoding.DecodeString(string(credentials["auth"]))

			if err != nil {
				log.Info("Cannot base64 decode auth")
				return nil, err
			}

			username, password, found := strings.Cut(string(auth_plain), ":")

			if !found {
				log.Info("Could not find delimiter in auth")
				return nil, errors.New("Could not find delimiter in auth")
			}

			hostRule["username"] = username
			hostRule["password"] = password
			hostRule["hostType"] = "docker"
		}

		hostRules = append(hostRules, hostRule)
	}

	return hostRules, nil
}

func (c *BaseComponent) GetRenovateBaseConfig(ctx context.Context, client client.Client) (map[string]interface{}, error) {
	renovateBaseConfigMutex.RLock()
	if renovateBaseConfig != nil {
		defer renovateBaseConfigMutex.RUnlock()
		return renovateBaseConfig, nil
	}
	renovateBaseConfigMutex.RUnlock()

	baseConfig := corev1.ConfigMap{}
	configmapKey := types.NamespacedName{Namespace: "mintmaker", Name: "renovate-config"}
	if err := client.Get(ctx, configmapKey, &baseConfig); err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(baseConfig.Data["renovate.json"]), &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling Renovate config: %v", err)
	}
	var selfHostedConfig map[string]interface{}
	if err := json.Unmarshal([]byte(baseConfig.Data["self_hosted.json"]), &selfHostedConfig); err != nil {
		return nil, fmt.Errorf("error unmarshaling Renovate self-hosted config: %v", err)
	}

	for k, v := range selfHostedConfig {
		config[k] = v
	}

	renovateBaseConfigMutex.Lock()
	renovateBaseConfig = config
	renovateBaseConfigMutex.Unlock()
	return config, nil
}

func getActivationKeyFromSecret(secret *corev1.Secret) (string, string, error) {

	if secret == nil {
		return "", "", fmt.Errorf("no viable activation key secret has been found")
	}

	activationKey := string(secret.Data["activationkey"])
	org := string(secret.Data["org"])

	if activationKey == "" || org == "" {
		return "", "", fmt.Errorf("secret %s doesn't contain activation key or org", secret.Name)
	}

	return activationKey, org, nil
}

func getRPMSSLAuthFromSecret(secret *corev1.Secret) (string, string, error) {

	if secret == nil {
		return "", "", fmt.Errorf("no viable rpm ssl auth secret has been found")
	}

	sslCert := string(secret.Data["sslcert"])
	sslKey := string(secret.Data["sslkey"])

	if sslCert == "" || sslKey == "" {
		return "", "", fmt.Errorf("secret %s doesn't contain sslcert or sslkey", secret.Name)
	}

	return sslCert, sslKey, nil
}

// returns two strings, activationkey and org
func (c *BaseComponent) GetRPMActivationKey(k8sClient client.Client, ctx context.Context) (string, string, error) {

	defaultSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: "activation-key"}, defaultSecret); err != nil {
		defaultSecret = &corev1.Secret{}
	}

	secretList := &corev1.SecretList{}
	opts := client.ListOption(&client.MatchingLabels{
		"appstudio.redhat.com/credentials": "rpm",
		"appstudio.redhat.com/scm.host":    c.Host,
	})

	// find secrets that have the following labels:
	//	- "appstudio.redhat.com/credentials": "rpm"
	//	- "appstudio.redhat.com/scm.host": <name of component host>
	if err := k8sClient.List(ctx, secretList, client.InNamespace(c.Namespace), opts); err != nil {
		return getActivationKeyFromSecret(defaultSecret)
	}

	// filtering to get Opaque secrets and data is not empty
	secrets := bslices.Filter(secretList.Items, func(secret corev1.Secret) bool {
		return secret.Type == corev1.SecretTypeOpaque && len(secret.Data) > 0
	})
	if len(secrets) == 0 {
		return getActivationKeyFromSecret(defaultSecret)
	}

	// secrets only match with component's host
	var hostOnlySecrets []corev1.Secret
	// map of secret index and its best path intersections count, i.e. the count of path parts matched,
	var potentialMatches = make(map[int]int, len(secrets))

	for index, secret := range secrets {
		repositoryLabel, exists := secret.Annotations["appstudio.redhat.com/scm.repository"]
		if !exists || repositoryLabel == "" {
			hostOnlySecrets = append(hostOnlySecrets, secret)
			continue
		}

		secretRepositories := strings.Split(repositoryLabel, ",")
		// trim possible prefix or suffix "/"
		for i, repository := range secretRepositories {
			secretRepositories[i] = strings.TrimPrefix(strings.TrimSuffix(repository, "/"), "/")
		}

		// this secret matches exactly the component's repository name
		if slices.Contains(secretRepositories, c.Repository) {
			return getActivationKeyFromSecret(&secret)
		}

		// no direct match, check for wildcard match, i.e. org/repo/* matches org/repo/foo, org/repo/bar, etc.
		componentRepoParts := strings.Split(c.Repository, "/")

		// find wildcard repositories
		wildcardRepos := slices.Filter(nil, secretRepositories, func(s string) bool { return strings.HasSuffix(s, "*") })

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
			// no potential matches, no host matches, try default secret if it exists
			return getActivationKeyFromSecret(defaultSecret)
		}
		// no potential matches, but we have host match secrets, return the first one
		return getActivationKeyFromSecret(&hostOnlySecrets[0])
	}

	// some potential matches exist, find the best one
	var bestIndex, bestCount int
	for i, count := range potentialMatches {
		if count > bestCount {
			bestCount = count
			bestIndex = i
		}
	}
	return getActivationKeyFromSecret(&secrets[bestIndex])

}

// returns two strings, sslcert and sslkey
func (c *BaseComponent) GetRPMSSLAuth(k8sClient client.Client, ctx context.Context) (string, string, error) {

	defaultSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: "rpm-ssl-auth"}, defaultSecret); err != nil {
		defaultSecret = &corev1.Secret{}
	}

	secretList := &corev1.SecretList{}
	opts := client.ListOption(&client.MatchingLabels{
		"appstudio.redhat.com/credentials": "rpm-ssl-auth",
		"appstudio.redhat.com/scm.host":    c.Host,
	})

	// find secrets that have the following labels:
	//	- "appstudio.redhat.com/credentials": "rpm-ssl-auth"
	//	- "appstudio.redhat.com/scm.host": <name of component host>
	if err := k8sClient.List(ctx, secretList, client.InNamespace(c.Namespace), opts); err != nil {
		return getRPMSSLAuthFromSecret(defaultSecret)
	}

	// filtering to get Opaque secrets and data is not empty
	secrets := bslices.Filter(secretList.Items, func(secret corev1.Secret) bool {
		return secret.Type == corev1.SecretTypeOpaque && len(secret.Data) > 0
	})
	if len(secrets) == 0 {
		return getRPMSSLAuthFromSecret(defaultSecret)
	}

	// secrets only match with component's host
	var hostOnlySecrets []corev1.Secret
	// map of secret index and its best path intersections count, i.e. the count of path parts matched,
	var potentialMatches = make(map[int]int, len(secrets))

	for index, secret := range secrets {
		repositoryLabel, exists := secret.Annotations["appstudio.redhat.com/scm.repository"]
		if !exists || repositoryLabel == "" {
			hostOnlySecrets = append(hostOnlySecrets, secret)
			continue
		}

		secretRepositories := strings.Split(repositoryLabel, ",")
		// trim possible prefix or suffix "/"
		for i, repository := range secretRepositories {
			secretRepositories[i] = strings.TrimPrefix(strings.TrimSuffix(repository, "/"), "/")
		}

		// this secret matches exactly the component's repository name
		if slices.Contains(secretRepositories, c.Repository) {
			return getRPMSSLAuthFromSecret(&secret)
		}

		// no direct match, check for wildcard match, i.e. org/repo/* matches org/repo/foo, org/repo/bar, etc.
		componentRepoParts := strings.Split(c.Repository, "/")

		// find wildcard repositories
		wildcardRepos := slices.Filter(nil, secretRepositories, func(s string) bool { return strings.HasSuffix(s, "*") })

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
			// no potential matches, no host matches, try default secret if it exists
			return getRPMSSLAuthFromSecret(defaultSecret)
		}
		// no potential matches, but we have host match secrets, return the first one
		return getRPMSSLAuthFromSecret(&hostOnlySecrets[0])
	}

	// some potential matches exist, find the best one
	var bestIndex, bestCount int
	for i, count := range potentialMatches {
		if count > bestCount {
			bestCount = count
			bestIndex = i
		}
	}
	return getRPMSSLAuthFromSecret(&secrets[bestIndex])

}
