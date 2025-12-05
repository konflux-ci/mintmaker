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

package config

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/konflux-ci/mintmaker/internal/pkg/constant"
)

const ConfigMapName = "mintmaker-controller-configmap"

type GlobalConfig struct {
	GhTokenValidity       time.Duration
	GhTokenUsageWindow    time.Duration
	GhTokenRenewThreshold time.Duration
	KiteAPIURL            string
}

type ControllerConfig struct {
	GlobalConfig GlobalConfig
}

var globalConfig *ControllerConfig
var once sync.Once

func DefaultConfig() *ControllerConfig {
	GhTokenValidity := 60 * time.Minute
	GhTokenUsageWindow := 30 * time.Minute

	return &ControllerConfig{
		GlobalConfig: GlobalConfig{
			GhTokenValidity:       GhTokenValidity,
			GhTokenUsageWindow:    GhTokenUsageWindow,
			GhTokenRenewThreshold: GhTokenValidity - GhTokenUsageWindow,
			KiteAPIURL:            os.Getenv("KITE_API_URL"),
		},
	}
}

func LoadConfig(ctx context.Context, client client.Reader) *ControllerConfig {
	log := ctrllog.FromContext(ctx).WithName("ConfigLoader")
	var configReader struct {
		Global struct {
			GhTokenValidity    string `json:"github-token-validity"`
			GhTokenUsageWindow string `json:"github-token-usage-window"`
			KiteAPIURL         string `json:"kite-api-url"`
		} `json:"global"`
	}

	defaultConfig := DefaultConfig()
	config := &ControllerConfig{}

	configMap := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: constant.MintMakerNamespaceName,
		Name:      ConfigMapName,
	}, configMap)
	if err != nil {
		log.Error(err, "ConfigMap not found, using default configuration", "configMap", ConfigMapName)
		return defaultConfig
	}

	if err := json.Unmarshal([]byte(configMap.Data["config.json"]), &configReader); err != nil {
		log.Error(err, "Could not unmarshal configuration, using default configuration", "configMap", ConfigMapName)
		return defaultConfig
	}

	if parsed, err := time.ParseDuration(configReader.Global.GhTokenValidity); err == nil && parsed > 0 {
		config.GlobalConfig.GhTokenValidity = parsed
	} else {
		config.GlobalConfig.GhTokenValidity = defaultConfig.GlobalConfig.GhTokenValidity
	}

	if parsed, err := time.ParseDuration(configReader.Global.GhTokenUsageWindow); err == nil && parsed > 0 {
		config.GlobalConfig.GhTokenUsageWindow = parsed
	} else {
		config.GlobalConfig.GhTokenUsageWindow = defaultConfig.GlobalConfig.GhTokenUsageWindow
	}

	if config.GlobalConfig.GhTokenUsageWindow >= config.GlobalConfig.GhTokenValidity {
		config.GlobalConfig.GhTokenValidity = defaultConfig.GlobalConfig.GhTokenValidity
		config.GlobalConfig.GhTokenUsageWindow = defaultConfig.GlobalConfig.GhTokenUsageWindow
		log.Error(err, "Invalid value for GitHub token usage window, using default",
			"default", defaultConfig.GlobalConfig.GhTokenUsageWindow)
	}

	config.GlobalConfig.GhTokenRenewThreshold = config.GlobalConfig.GhTokenValidity - config.GlobalConfig.GhTokenUsageWindow

	if configReader.Global.KiteAPIURL != "" {
		config.GlobalConfig.KiteAPIURL = configReader.Global.KiteAPIURL
	} else {
		// Fallback to environment variable
		config.GlobalConfig.KiteAPIURL = os.Getenv("KITE_API_URL")
	}

	return config
}

// Will not return empty configs but error for logging purposses
func InitGlobalConfig(ctx context.Context, client client.Reader) {
	once.Do(func() {
		config := LoadConfig(ctx, client)
		globalConfig = config
	})
}

func GetConfig() *ControllerConfig {
	if globalConfig == nil {
		return DefaultConfig()
	}
	return globalConfig
}

// Get testing config
func GetTestConfig() *ControllerConfig {
	return DefaultConfig()
}
