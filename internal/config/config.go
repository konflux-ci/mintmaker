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

// Package config provides configuration management for the MintMaker controller.
//
// Configuration is loaded from a JSON file. The file path can be specified
// via the MINTMAKER_CONFIG_PATH environment variable, defaulting to
// /etc/mintmaker/config.json.
//
// Example config.json:
//
//	{
//	  "github": {
//	    "token-ttl": "60m",
//	    "token-min-validity": "30m"
//	  },
//	  "kite": {
//	    "enabled": true,
//	    "api-url": "https://kite.example.com"
//	  }
//	}
//
// GitHub Token Configuration:
//
// GitHub installation tokens have a limited lifetime (token-ttl). To ensure
// tokens remain valid throughout their usage, we renew them before they expire.
// The token-min-validity specifies the minimum remaining validity required
// for a token to be considered usable.
//
// Example: With token-ttl=60m and token-min-validity=30m:
//   - Token created at 20:00, expires at 21:00
//   - At 20:25 (35m remaining > 30m min): token is usable
//   - At 20:35 (25m remaining < 30m min): token needs renewal
//
// Kite Configuration:
//
// Kite integration reports issues to Kite after analyzing Renovate logs.
// It is disabled by default.
//
//   - enabled: Set to true to enable Kite integration. When enabled, a
//     log-analyzer step is added to the pipelinerun. Defaults to false.
//   - api-url: The URL of the Kite API endpoint. Can also be set via
//     KITE_API_URL environment variable (config file takes precedence).
package config

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	defaultConfigPath       = "/etc/mintmaker/config.json"
	configPathEnvVar        = "MINTMAKER_CONFIG_PATH"
	defaultTokenTTL         = 60 * time.Minute
	defaultTokenMinValidity = 30 * time.Minute
)

// GitHubConfig holds GitHub-related configuration.
type GitHubConfig struct {
	// TokenTTL is the total validity period of a GitHub installation token
	// from the time it is created. GitHub installation tokens typically
	// expire after 1 hour.
	TokenTTL time.Duration

	// TokenMinValidity is the minimum remaining validity required for a token
	// to be considered usable. When a token's remaining lifetime falls below
	// this value, it should be renewed.
	//
	// For example, with TokenTTL=60m and TokenMinValidity=30m:
	// - A token with 35 minutes remaining is usable
	// - A token with 25 minutes remaining needs renewal
	TokenMinValidity time.Duration
}

// KiteConfig holds Kite-related configuration.
type KiteConfig struct {
	// Enabled controls whether Kite integration is active.
	// When enabled, a log-analyzer step is added to the pipeline to send
	// Renovate logs to the Kite API. Defaults to false.
	Enabled bool

	// APIURL is the URL of the Kite API. Required when Enabled is true.
	APIURL string
}

// Config holds all controller configuration.
type Config struct {
	GitHub GitHubConfig
	Kite   KiteConfig
}

// fileConfig represents the JSON structure of the config file.
type fileConfig struct {
	GitHub struct {
		TokenTTL         string `json:"token-ttl"`
		TokenMinValidity string `json:"token-min-validity"`
	} `json:"github"`
	Kite struct {
		Enabled bool   `json:"enabled"`
		APIURL  string `json:"api-url"`
	} `json:"kite"`
}

var (
	instance *Config
	once     sync.Once
)

// defaultConfig returns a Config with default values.
func defaultConfig() *Config {
	return &Config{
		GitHub: GitHubConfig{
			TokenTTL:         defaultTokenTTL,
			TokenMinValidity: defaultTokenMinValidity,
		},
		Kite: KiteConfig{
			Enabled: false,
			APIURL:  os.Getenv("KITE_API_URL"),
		},
	}
}

// load reads configuration from a file and returns a Config.
// If the file doesn't exist or is invalid, it returns default configuration.
func load(path string) *Config {
	log := ctrllog.Log.WithName("config")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("config file not found, using defaults", "path", path)
		} else {
			log.Error(err, "failed to read config file, using defaults", "path", path)
		}
		return defaultConfig()
	}

	return parse(data, log)
}

// parse unmarshals and validates the config data.
func parse(data []byte, log logr.Logger) *Config {
	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		log.Error(err, "failed to parse config file, using defaults")
		return defaultConfig()
	}

	cfg := defaultConfig()

	// GitHub token config
	if ttl, err := time.ParseDuration(fc.GitHub.TokenTTL); err == nil && ttl > 0 {
		cfg.GitHub.TokenTTL = ttl
	}

	if minValidity, err := time.ParseDuration(fc.GitHub.TokenMinValidity); err == nil && minValidity > 0 {
		cfg.GitHub.TokenMinValidity = minValidity
	}

	// Kite config: file takes precedence over env var
	cfg.Kite.Enabled = fc.Kite.Enabled
	if fc.Kite.APIURL != "" {
		cfg.Kite.APIURL = fc.Kite.APIURL
	}

	if err := cfg.validate(log); err != nil {
		return defaultConfig()
	}
	return cfg
}

// validate checks that the configuration values are valid.
func (c *Config) validate(log logr.Logger) error {
	if c.GitHub.TokenMinValidity >= c.GitHub.TokenTTL {
		log.Info("invalid config: token-min-validity must be less than token-ttl, using defaults",
			"token-ttl", c.GitHub.TokenTTL,
			"token-min-validity", c.GitHub.TokenMinValidity)
		return errInvalidConfig
	}
	return nil
}

var errInvalidConfig = &configError{"invalid configuration"}

type configError struct {
	msg string
}

func (e *configError) Error() string {
	return e.msg
}

// Get returns the global configuration.
// On first call, it loads the config from file (path from MINTMAKER_CONFIG_PATH
// env var, or /etc/mintmaker/config.json by default).
func Get() *Config {
	once.Do(func() {
		path := os.Getenv(configPathEnvVar)
		if path == "" {
			path = defaultConfigPath
		}
		instance = load(path)
	})
	return instance
}
