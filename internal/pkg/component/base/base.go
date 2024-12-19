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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
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
	Timestamp  int64
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

func (c *BaseComponent) GetTimestamp() int64 {
	return c.Timestamp
}

type HostRule map[string]string

func (c *BaseComponent) TransformHostRules(ctx context.Context, registrySecret *corev1.Secret) ([]HostRule, error) {
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

func (c *BaseComponent) GetRenovateBaseConfig(client client.Client, ctx context.Context, registrySecret *corev1.Secret) (map[string]interface{}, error) {

	if renovateBaseConfig != nil {
		return renovateBaseConfig, nil
	}

	baseConfig := corev1.ConfigMap{}
	configmapKey := types.NamespacedName{Namespace: "mintmaker", Name: "renovate-config"}
	if err := client.Get(ctx, configmapKey, &baseConfig); err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(baseConfig.Data["renovate.json"]), &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling Renovate config: %v", err)
	}

	if registrySecret != nil {
		hostRules, err := c.TransformHostRules(ctx, registrySecret)

		if err == nil {
			config["hostRules"] = hostRules
		}
	}

	renovateBaseConfigMutex.Lock()
	renovateBaseConfig = config
	renovateBaseConfigMutex.Unlock()
	return config, nil
}
