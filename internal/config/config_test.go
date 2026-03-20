package config

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestParse(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name                string
		data                string
		expectedTokenTTL    time.Duration
		expectedMinValidity time.Duration
		expectedKiteEnabled bool
		expectedKiteAPIURL  string
	}{
		{
			name: "valid full config",
			data: `{
				"github": {
					"token-ttl": "45m",
					"token-min-validity": "15m"
				},
				"kite": {
					"enabled": true,
					"api-url": "https://kite.example.com"
				}
			}`,
			expectedTokenTTL:    45 * time.Minute,
			expectedMinValidity: 15 * time.Minute,
			expectedKiteEnabled: true,
			expectedKiteAPIURL:  "https://kite.example.com",
		},
		{
			name:                "empty JSON uses defaults",
			data:                `{}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: defaultTokenMinValidity,
			expectedKiteEnabled: false,
			expectedKiteAPIURL:  "",
		},
		{
			name:                "invalid JSON uses defaults",
			data:                `{invalid`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: defaultTokenMinValidity,
			expectedKiteEnabled: false,
			expectedKiteAPIURL:  "",
		},
		{
			name: "invalid duration uses default",
			data: `{
				"github": {
					"token-ttl": "notaduration",
					"token-min-validity": "15m"
				}
			}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: 15 * time.Minute,
			expectedKiteEnabled: false,
		},
		{
			name: "negative duration uses default",
			data: `{
				"github": {
					"token-ttl": "-10m",
					"token-min-validity": "15m"
				}
			}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: 15 * time.Minute,
			expectedKiteEnabled: false,
		},
		{
			name: "min validity >= ttl falls back to defaults",
			data: `{
				"github": {
					"token-ttl": "30m",
					"token-min-validity": "30m"
				}
			}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: defaultTokenMinValidity,
			expectedKiteEnabled: false,
		},
		{
			name: "min validity > ttl falls back to defaults",
			data: `{
				"github": {
					"token-ttl": "30m",
					"token-min-validity": "45m"
				}
			}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: defaultTokenMinValidity,
			expectedKiteEnabled: false,
		},
		{
			name: "only github config",
			data: `{
				"github": {
					"token-ttl": "90m",
					"token-min-validity": "20m"
				}
			}`,
			expectedTokenTTL:    90 * time.Minute,
			expectedMinValidity: 20 * time.Minute,
			expectedKiteEnabled: false,
		},
		{
			name: "only kite config",
			data: `{
				"kite": {
					"enabled": true,
					"api-url": "https://kite.test.com"
				}
			}`,
			expectedTokenTTL:    defaultTokenTTL,
			expectedMinValidity: defaultTokenMinValidity,
			expectedKiteEnabled: true,
			expectedKiteAPIURL:  "https://kite.test.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := parse([]byte(tc.data), log)

			if cfg.GitHub.TokenTTL != tc.expectedTokenTTL {
				t.Errorf("TokenTTL: expected %v, got %v", tc.expectedTokenTTL, cfg.GitHub.TokenTTL)
			}
			if cfg.GitHub.TokenMinValidity != tc.expectedMinValidity {
				t.Errorf("TokenMinValidity: expected %v, got %v", tc.expectedMinValidity, cfg.GitHub.TokenMinValidity)
			}
			if cfg.Kite.Enabled != tc.expectedKiteEnabled {
				t.Errorf("Kite.Enabled: expected %v, got %v", tc.expectedKiteEnabled, cfg.Kite.Enabled)
			}
			if tc.expectedKiteAPIURL != "" && cfg.Kite.APIURL != tc.expectedKiteAPIURL {
				t.Errorf("Kite.APIURL: expected %q, got %q", tc.expectedKiteAPIURL, cfg.Kite.APIURL)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.GitHub.TokenTTL != defaultTokenTTL {
		t.Errorf("expected default TokenTTL %v, got %v", defaultTokenTTL, cfg.GitHub.TokenTTL)
	}
	if cfg.GitHub.TokenMinValidity != defaultTokenMinValidity {
		t.Errorf("expected default TokenMinValidity %v, got %v", defaultTokenMinValidity, cfg.GitHub.TokenMinValidity)
	}
	if cfg.Kite.Enabled {
		t.Error("expected Kite.Enabled to be false by default")
	}
}

func TestValidate(t *testing.T) {
	log := logr.Discard()

	tests := []struct {
		name        string
		cfg         Config
		expectError bool
	}{
		{
			name: "valid config",
			cfg: Config{
				GitHub: GitHubConfig{
					TokenTTL:         60 * time.Minute,
					TokenMinValidity: 30 * time.Minute,
				},
			},
			expectError: false,
		},
		{
			name: "min validity equals ttl",
			cfg: Config{
				GitHub: GitHubConfig{
					TokenTTL:         60 * time.Minute,
					TokenMinValidity: 60 * time.Minute,
				},
			},
			expectError: true,
		},
		{
			name: "min validity exceeds ttl",
			cfg: Config{
				GitHub: GitHubConfig{
					TokenTTL:         30 * time.Minute,
					TokenMinValidity: 60 * time.Minute,
				},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate(log)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	// Test loading from non-existent file returns defaults
	cfg := load("/nonexistent/path/config.json")
	if cfg.GitHub.TokenTTL != defaultTokenTTL {
		t.Errorf("expected default TokenTTL, got %v", cfg.GitHub.TokenTTL)
	}
	if cfg.GitHub.TokenMinValidity != defaultTokenMinValidity {
		t.Errorf("expected default TokenMinValidity, got %v", cfg.GitHub.TokenMinValidity)
	}
}

func TestConfigError(t *testing.T) {
	err := &configError{msg: "test error"}
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}
}
