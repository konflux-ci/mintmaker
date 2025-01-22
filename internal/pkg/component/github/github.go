package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v45/github"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/pkg/component/base"
	"github.com/konflux-ci/mintmaker/internal/pkg/utils"
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//TODO: doc about only supporting GitHub with the installed GitHub App

var (
	ghAppInstallationsCache          *Cache
	ghAppInstallationsCacheMutex     sync.Mutex
	ghAppInstallationsCacheID        int64
	ghAppInstallationTokenCache      *Cache
	ghAppInstallationTokenCacheMutex sync.Mutex
	ghAppInstallationTokenCacheID    int64
	ghAppID                          int64
	ghAppPrivateKey                  []byte
)

type AppInstallation struct {
	InstallationID int64
	Repositories   []string
}

type Component struct {
	base.BaseComponent
	AppID         int64
	AppPrivateKey []byte
	client        client.Client
	ctx           context.Context
}

func getAppIDAndKey(client client.Client, ctx context.Context) (int64, []byte, error) {
	if ghAppID != 0 && ghAppPrivateKey != nil {
		return ghAppID, ghAppPrivateKey, nil
	}
	//Check if GitHub Application is used, if not then skip
	appSecret := corev1.Secret{}
	appSecretKey := types.NamespacedName{Namespace: "mintmaker", Name: "pipelines-as-code-secret"}
	if err := client.Get(ctx, appSecretKey, &appSecret); err != nil {
		return 0, nil, err
	}

	// validate content of the fields
	num, err := strconv.ParseInt(string(appSecret.Data["github-application-id"]), 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse GitHub APP ID: %w", err)
	}
	ghAppID = num
	ghAppPrivateKey = appSecret.Data["github-private-key"]
	return ghAppID, ghAppPrivateKey, nil
}

func NewComponent(comp *appstudiov1alpha1.Component, timestamp int64, client client.Client, ctx context.Context) (*Component, error) {
	appID, appPrivateKey, err := getAppIDAndKey(client, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub APP ID and private key: %w", err)
	}
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
		AppID:         appID,
		AppPrivateKey: appPrivateKey,
		client:        client,
		ctx:           ctx,
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

func (c *Component) GetToken() (string, error) {
	ghAppInstallationTokenCacheMutex.Lock()
	defer ghAppInstallationTokenCacheMutex.Unlock()

	if ghAppInstallationTokenCache == nil || ghAppInstallationTokenCacheID != c.Timestamp {
		ghAppInstallationTokenCache = NewCache()
		ghAppInstallationTokenCacheID = c.Timestamp
	}

	appInstallations, err := c.getAppInstallations()
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub App installations: %w", err)
	}

	var installationID int64
	found := false

	for _, installation := range appInstallations {
		for _, repo := range installation.Repositories {
			repo = strings.TrimSuffix(strings.TrimPrefix(repo, "/"), "/")
			if repo == c.Repository {
				installationID = installation.InstallationID
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return "", fmt.Errorf("repository %s not found in any GitHub App installation", c.Repository)
	}

	token_key := fmt.Sprintf("installation_%d", installationID)
	if token, ok := ghAppInstallationTokenCache.Get(token_key); ok {
		return token.(string), nil
	}

	itr, err := ghinstallation.New(
		http.DefaultTransport,
		c.AppID,
		installationID,
		c.AppPrivateKey,
	)
	if err != nil {
		return "", fmt.Errorf("error creating installation transport: %w", err)
	}

	token, err := itr.Token(context.Background())
	if err != nil {
		return "", fmt.Errorf("error getting installation token: %w", err)
	}
	ghAppInstallationTokenCache.Set(token_key, token)
	return token, nil
}

func (c *Component) getAppInstallations() ([]AppInstallation, error) {
	ghAppInstallationsCacheMutex.Lock()
	defer ghAppInstallationsCacheMutex.Unlock()

	if ghAppInstallationsCache == nil || ghAppInstallationsCacheID != c.Timestamp {
		ghAppInstallationsCache = NewCache()
		ghAppInstallationsCacheID = c.Timestamp
	}
	if data, ok := ghAppInstallationsCache.Get("installations"); ok {
		return data.([]AppInstallation), nil
	}

	var appInstallations []AppInstallation

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, c.AppID, c.AppPrivateKey)
	if err != nil {
		return nil, err
	}

	client := github.NewClient(&http.Client{Transport: itr})
	_, _, err = client.Apps.Get(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to load GitHub app metadata, %w", err)
	}

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		installations, resp, err := client.Apps.ListInstallations(context.Background(), &opt.ListOptions)
		if err != nil {
			if resp != nil && resp.Response != nil && resp.Response.StatusCode != 0 {
				switch resp.StatusCode {
				case 401:
					return nil, fmt.Errorf("GitHub Application private key does not match Application ID")
				case 404:
					return nil, fmt.Errorf("GitHub Application with given ID does not exist")
				}
			}
			return nil, fmt.Errorf("error getting GitHub Application installations: %w", err)
		}
		for _, installation := range installations {
			appInstall := AppInstallation{
				InstallationID: installation.GetID(),
			}

			itr, err := ghinstallation.New(http.DefaultTransport, c.AppID, installation.GetID(), c.AppPrivateKey)
			if err != nil {
				return nil, fmt.Errorf("error creating installation transport: %w", err)
			}

			installationClient := github.NewClient(&http.Client{Transport: itr})
			repoOpt := &github.ListOptions{PerPage: 100}
			for {
				repos, repoResp, err := installationClient.Apps.ListRepos(context.Background(), repoOpt)
				if err != nil {
					// If App is installed with insufficient permission, this ListRepos call
					// will return error, we should just skip checking this installation
					// TODO: error logging
					break
				}
				for _, repo := range repos.Repositories {
					appInstall.Repositories = append(appInstall.Repositories, repo.GetFullName())
				}
				if repoResp.NextPage == 0 {
					break
				}
				repoOpt.Page = repoResp.NextPage
			}
			appInstallations = append(appInstallations, appInstall)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	ghAppInstallationsCache.Set("installations", appInstallations)
	return appInstallations, nil
}

func (c *Component) getDefaultBranch() (string, error) {
	token, err := c.GetToken()
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub token: %w", err)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)
	parts := strings.Split(c.Repository, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repository format: %s", c.Repository)
	}
	owner := parts[0]
	repo := parts[1]
	repositoryInfo, _, err := client.Repositories.Get(context.Background(), owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get repository information: %w", err)
	}
	if repositoryInfo.DefaultBranch == nil {
		return "", fmt.Errorf("repository default branch is nil")
	}
	return *repositoryInfo.DefaultBranch, nil
}

func (c *Component) GetAPIEndpoint() string {
	return fmt.Sprintf("https://api.%s/", c.Host)
}

func (c *Component) getAppSlug() (string, error) {
	appID, appPrivateKey, err := getAppIDAndKey(c.client, c.ctx)
	if err != nil {
		return "", err
	}
	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, appPrivateKey)
	if err != nil {
		return "", err
	}

	client := github.NewClient(&http.Client{Transport: itr})
	app, _, err := client.Apps.Get(context.Background(), "")
	if err != nil {
		return "", fmt.Errorf("failed to load GitHub app metadata, %w", err)
	}
	slug := app.GetSlug()
	return slug, nil
}

func (c *Component) GetRenovateConfig(registrySecret *corev1.Secret) (string, error) {
	baseConfig, err := c.GetRenovateBaseConfig(c.client, c.ctx, registrySecret)
	if err != nil {
		return "", err
	}
	appSlug, err := c.getAppSlug()
	if err != nil {
		return "", err
	}
	baseConfig["platform"] = c.Platform
	baseConfig["endpoint"] = c.GetAPIEndpoint()
	baseConfig["username"] = fmt.Sprintf("%s[bot]", appSlug)
	baseConfig["gitAuthor"] = fmt.Sprintf("%s <126015336+%s[bot]@users.noreply.github.com>", appSlug, appSlug)

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
		return "", err
	}
	return string(updatedConfig), nil
}
