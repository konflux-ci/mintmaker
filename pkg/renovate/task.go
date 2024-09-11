package renovate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	. "github.com/konflux-ci/mintmaker/pkg/common"
	"github.com/konflux-ci/mintmaker/pkg/git"
	"github.com/konflux-ci/mintmaker/pkg/git/credentials"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Task represents a task to be executed by Renovate with credentials and repositories
type Task struct {
	Platform     string
	Username     string
	GitAuthor    string
	Token        string
	Endpoint     string
	Repositories []*Repository
}

// AddNewBranchToTheExistedRepositoryTasksOnTheSameHosts iterates over the tasks and adds a new branch to the repository if it already exists
// NOTE: performing this operation on a slice containing tasks from different platforms or hosts is unsafe.
func AddNewBranchToTheExistedRepositoryTasksOnTheSameHosts(tasks []*Task, component *git.ScmComponent) bool {
	for _, t := range tasks {
		for _, r := range t.Repositories {
			if r.Repository == component.Repository() {
				r.AddBranch(component.Branch())
				return true
			}
		}
	}
	return false
}

// AddNewRepoToTasksOnTheSameHostsWithSameCredentials iterates over the tasks and adds a new repository to the task with same credentials
// NOTE: performing this operation on a slice containing tasks from different platforms or hosts is unsafe.
func AddNewRepoToTasksOnTheSameHostsWithSameCredentials(tasks []*Task, component *git.ScmComponent, cred *credentials.BasicAuthCredentials) bool {
	for _, t := range tasks {
		if t.Token == cred.Password && t.Username == cred.Username {
			//double check if the repository is already added
			for _, r := range t.Repositories {
				if r.Repository == component.Repository() {
					return true
				}
			}
			t.Repositories = append(t.Repositories, &Repository{
				Repository:   component.Repository(),
				BaseBranches: []string{component.Branch()},
			})
			return true
		}
	}
	return false
}

// TaskProvider is an interface for providing tasks to be executed by Renovate
type TaskProvider interface {
	GetNewTasks(ctx context.Context, components []*git.ScmComponent) []*Task
}

type HostRule map[string]string

func (t *Task) TransformHostRules(ctx context.Context, registrySecret *corev1.Secret) ([]HostRule, error) {
	log := logger.FromContext(ctx)

	if registrySecret == nil {
		return nil, errors.New("Registry secret is nil")
	}

	var hostRules []HostRule
	var secrets map[string]map[string]HostRule

	err := json.Unmarshal(registrySecret.Data[".dockerconfigjson"], &secrets)

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

func (t *Task) GetJobConfig(ctx context.Context, client client.Client, registrySecret *corev1.Secret) (string, error) {
	defaultConfig := corev1.ConfigMap{}
	renovateDefaultConfig := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: RenovateConfigMapName}
	if err := client.Get(ctx, renovateDefaultConfig, &defaultConfig); err != nil {
		return "", err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(defaultConfig.Data[RenovateConfigKey]), &config); err != nil {
		return "", fmt.Errorf("error unmarshaling Renovate config: %v", err)
	}

	config["platform"] = t.Platform
	config["endpoint"] = t.Endpoint
	config["username"] = t.Username
	config["gitAuthor"] = t.GitAuthor

	if registrySecret != nil {
		hostRules, err := t.TransformHostRules(ctx, registrySecret)

		if err == nil {
			config["hostRules"] = hostRules
		}
	}

	repos, _ := json.Marshal(t.Repositories)
	var repoData []map[string]interface{}
	if err := json.Unmarshal(repos, &repoData); err != nil {
		return "", fmt.Errorf("error unmarshaling repositories in task into interface: %v", err)
	}
	config["repositories"] = repoData

	updatedConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling updated Renovate config: %v", err)
	}

	return string(updatedConfig), nil
}
