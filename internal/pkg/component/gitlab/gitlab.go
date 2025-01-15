package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/pkg/component/base"
	bslices "github.com/konflux-ci/mintmaker/internal/pkg/slices"
	"github.com/konflux-ci/mintmaker/internal/pkg/utils"
	"github.com/xanzy/go-gitlab"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func NewComponent(comp *appstudiov1alpha1.Component, timestamp int64, client client.Client, ctx context.Context) (*Component, error) {
	giturl := comp.Spec.Source.GitSource.URL
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
			Name:        comp.Name,
			Namespace:   comp.Namespace,
			Application: comp.Spec.Application,
			Platform:    platform,
			Host:        host,
			GitURL:      giturl,
			Repository:  repository,
			Timestamp:   timestamp,
			Branch:      comp.Spec.Source.GitSource.Revision,
		},
		client: client,
		ctx:    ctx,
	}, nil
}

func (c *Component) GetBranch() (string, error) {
	if c.Branch != "" {
		return c.Branch, nil
	}

	branch, err := c.getDefaultBranch()
	if err != nil {
		return "main", nil
	}
	return branch, nil
}

func (c *Component) lookupSecret() (*corev1.Secret, error) {

	secretList := &corev1.SecretList{}
	opts := client.ListOption(&client.MatchingLabels{
		"appstudio.redhat.com/credentials": "scm",
		"appstudio.redhat.com/scm.host":    c.Host,
	})

	// find secrets that have the following labels:
	//	- "appstudio.redhat.com/credentials": "scm"
	//	- "appstudio.redhat.com/scm.host": <name of component host>
	if err := c.client.List(c.ctx, secretList, client.InNamespace(c.Namespace), opts); err != nil {
		return nil, fmt.Errorf("failed to list scm secrets in namespace %s: %w", c.Namespace, err)
	}

	// filtering to get BasicAuth secrets and data is not empty
	secrets := bslices.Filter(secretList.Items, func(secret corev1.Secret) bool {
		return secret.Type == corev1.SecretTypeBasicAuth && len(secret.Data) > 0
	})
	if len(secrets) == 0 {
		return nil, fmt.Errorf("no secrets available for git host %s", c.Host)
	}

	// secrets only match with component's host
	var hostOnlySecrets []corev1.Secret
	// map of secret index and its best path intersections count, i.e. the count of path parts matched,
	var potentialMatches = make(map[int]int, len(secrets))

	for index, secret := range secrets {
		repositoryAnnotation, exists := secret.Annotations["appstudio.redhat.com/scm.repository"]
		if !exists || repositoryAnnotation == "" {
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
	token, err := c.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab token: %w", err)
	}
	u, err := url.Parse(c.GitURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse git url: %w", err)
	}
	baseUrl := u.Scheme + "://" + c.Host
	client, _ := gitlab.NewClient(token, gitlab.WithBaseURL(baseUrl))
	project, _, err := client.Projects.GetProject(c.Repository, nil)
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", fmt.Errorf("project info is empty in GitLab API response")
	}
	return project.DefaultBranch, nil
}

func (c *Component) GetRenovateConfig(registrySecret *corev1.Secret) (string, error) {
	baseConfig, err := c.GetRenovateBaseConfig(c.client, c.ctx, registrySecret)
	if err != nil {
		return "", err
	}

	baseConfig["platform"] = c.Platform
	baseConfig["endpoint"] = c.GetAPIEndpoint()
	// We don't need to set a username or gitAuthor for gitlab, since this is tight to a token
	baseConfig["username"] = ""
	baseConfig["gitAuthor"] = ""

	// TODO: perhaps in the future let's validate all these values
	branch, err := c.GetBranch()
	if err != nil {
		return "", err
	}
	repo := map[string]interface{}{
		"baseBranches": []string{branch},
		"repository":   c.Repository,
	}
	baseConfig["repositories"] = []interface{}{repo}

	updatedConfig, err := json.MarshalIndent(baseConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling updated Renovate config: %v", err)
	}

	return string(updatedConfig), nil
}
