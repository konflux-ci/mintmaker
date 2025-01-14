package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v45/github"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/pkg/component/base"
	"github.com/konflux-ci/mintmaker/internal/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//TODO: doc about only supporting GitHub with the installed GitHub App

var (
	ghAppInstallationsCache              *Cache
	ghAppInstallationsCacheMutex         sync.Mutex
	ghAppInstallationsCacheID            int64
	ghAppInstallationTokenCache          TokenCache
	ghAppInstallationTokenTimeThreshold  = 30 * time.Minute
	ghAppInstallationTokenValidityWindow = time.Hour
	ghAppID                              int64
	ghAppPrivateKey                      []byte
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

type TokenInfo struct {
	Token     string
	ExpiresAt time.Time
}

type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]TokenInfo
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

func (c *TokenCache) Set(key string, tokenInfo TokenInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = tokenInfo
}

func (c *TokenCache) Get(key string) (TokenInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return TokenInfo{}, false
	}

	// when token is going to expire in 30m, we can't use it
	if time.Until(entry.ExpiresAt) < ghAppInstallationTokenTimeThreshold {
		return TokenInfo{}, false
	}

	return entry, true
}

func (c *Component) GetBranch() (string, error) {
	if c.Branch != "" {
		return c.Branch, nil
	}

	branch, err := c.getDefaultBranch()
	if err != nil {
		// TODO, handle the error
		return "main", nil
	}
	return branch, nil
}

func (c *Component) getMyInstallationID(appInstallations []AppInstallation) (int64, error) {
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
		return 0, fmt.Errorf("repository %s not found in any GitHub App installation", c.Repository)
	}
	return installationID, nil
}

func (c *Component) GetToken() (string, error) {

	appInstallations, err := c.getAppInstallations()
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub App installations: %w", err)
	}
	installationID, err := c.getMyInstallationID(appInstallations)
	if err != nil {
		return "", err
	}

	tokenKey := fmt.Sprintf("installation_%d", installationID)

	// when token exists and within the threshold, a valid token is returned
	if tokenInfo, ok := ghAppInstallationTokenCache.Get(tokenKey); ok {
		return tokenInfo.Token, nil
	}
	// when token doesn't exist or not within the threshold, we generate a new token and update the cache
	tokenInfo, err := c.generateNewToken(installationID)
	if err != nil {
		return "", err
	}

	ghAppInstallationTokenCache.Set(tokenKey, tokenInfo)
	return tokenInfo.Token, nil
}

func (c *Component) generateNewToken(installationID int64) (TokenInfo, error) {

	itr, err := ghinstallation.New(
		http.DefaultTransport,
		c.AppID,
		installationID,
		c.AppPrivateKey,
	)
	if err != nil {
		return TokenInfo{Token: "", ExpiresAt: time.Time{}}, fmt.Errorf("error creating installation transport: %w", err)
	}

	token, err := itr.Token(context.Background())
	if err != nil {
		return TokenInfo{Token: "", ExpiresAt: time.Time{}}, fmt.Errorf("error getting installation token: %w", err)
	}

	return TokenInfo{Token: token, ExpiresAt: time.Now().Add(ghAppInstallationTokenValidityWindow)}, nil

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
					return nil, fmt.Errorf("error listing repos for installation %d: %w", installation.GetID(), err)
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
	// TODO: call github APIs to determine the default branch
	return "main", nil
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
