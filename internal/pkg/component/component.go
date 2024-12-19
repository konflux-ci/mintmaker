package component

import (
	"context"
	"fmt"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	github "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
	gitlab "github.com/konflux-ci/mintmaker/internal/pkg/component/gitlab"
	utils "github.com/konflux-ci/mintmaker/internal/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GitComponent interface {
	GetName() string
	GetNamespace() string
	GetApplication() string
	GetPlatform() string
	GetHost() string
	GetGitURL() string
	GetRepository() string
	GetTimestamp() int64
	GetToken() (string, error)
	GetBranch() (string, error)
	GetAPIEndpoint() string
	GetRenovateConfig(*corev1.Secret) (string, error)
}

func NewGitComponent(comp *appstudiov1alpha1.Component, timestamp int64, client client.Client, ctx context.Context) (GitComponent, error) {
	// TODO: validate git URL
	platform, err := utils.GetGitPlatform(comp.Spec.Source.GitSource.URL)
	if err != nil {
		return nil, err
	}

	switch platform {
	case "github":
		c, err := github.NewComponent(comp, timestamp, client, ctx)
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	case "gitlab":
		c, err := gitlab.NewComponent(comp, timestamp, client, ctx)
		if err != nil {
			return nil, fmt.Errorf("error creating git component: %w", err)
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}
