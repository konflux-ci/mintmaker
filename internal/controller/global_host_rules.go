// Copyright 2026 Red Hat, Inc.
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

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

const (
	hostRulesSecretKey  = "host-rules.json" //nolint:gosec // Kubernetes Secret data key, not a credential
	hostRulesMountPath  = "/etc/renovate/host-rules"
	hostRulesSecretPath = hostRulesMountPath + "/" + hostRulesSecretKey
)

type globalHostRule struct {
	HostType  string `json:"hostType"`
	MatchHost string `json:"matchHost"`
	Token     string `json:"token,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
}

// validateGlobalHostRules checks the Secret without returning or logging its data.
// The hostType is intentionally not restricted to a list of known ecosystems.
func validateGlobalHostRules(data []byte) error {
	var rules []globalHostRule
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil || rules == nil {
		return fmt.Errorf("%s must be a JSON array of rules containing only hostType, matchHost, token, username, and password", hostRulesSecretKey)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%s must contain only one JSON array", hostRulesSecretKey)
	}
	var rawRules []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawRules); err != nil {
		return fmt.Errorf("%s must be a JSON array of rules", hostRulesSecretKey)
	}
	for i, rawRule := range rawRules {
		for field := range rawRule {
			switch field {
			case "hostType", "matchHost", "token", "username", "password":
			default:
				return fmt.Errorf("rule %d has an invalid field name", i)
			}
		}
	}
	for i, rule := range rules {
		if strings.TrimSpace(rule.HostType) != rule.HostType || rule.HostType == "" || strings.ContainsAny(rule.HostType, "\t\r\n ") {
			return fmt.Errorf("rule %d has an invalid hostType", i)
		}
		if !validMatchHost(rule.MatchHost) {
			return fmt.Errorf("rule %d has an invalid matchHost", i)
		}
		hasToken := rule.Token != ""
		hasBasic := rule.Username != "" || rule.Password != ""
		if hasToken == hasBasic || (hasBasic && (rule.Username == "" || rule.Password == "")) {
			return fmt.Errorf("rule %d must have either a token or a username and password", i)
		}
	}
	return nil
}

func validMatchHost(matchHost string) bool {
	if matchHost == "" || strings.TrimSpace(matchHost) != matchHost || strings.ContainsAny(matchHost, "*?#") {
		return false
	}
	hasScheme := strings.Contains(matchHost, "://")
	value := matchHost
	if !hasScheme {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return hasScheme || parsed.Path == "" || parsed.Path == "/"
}

func (r *DependencyUpdateCheckReconciler) validateHostRulesSecret(ctx context.Context, name string) error {
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: mmconst.MintMakerNamespaceName, Name: name}, secret); err != nil {
		return fmt.Errorf("cannot read global host rules Secret %q: %w", name, err)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		return fmt.Errorf("global host rules Secret %q must have type Opaque", name)
	}
	data, ok := secret.Data[hostRulesSecretKey]
	if !ok {
		return fmt.Errorf("global host rules Secret %q is missing %s", name, hostRulesSecretKey)
	}
	if err := validateGlobalHostRules(data); err != nil {
		return fmt.Errorf("global host rules Secret %q: %w", name, err)
	}
	return nil
}

// renovateConfigJS reads credentials only inside the Renovate container.
// The generated ConfigMap contains this loader and nonsecret per-run config.
func renovateConfigJS(configJSON string, withGlobalRules bool) string {
	js := "module.exports = " + configJSON
	if !withGlobalRules {
		return js
	}
	return js + ";\n" +
		"try {\n" +
		"  const rules = JSON.parse(require('node:fs').readFileSync('" + hostRulesSecretPath + "', 'utf8'));\n" +
		"  module.exports.hostRules = [...(module.exports.hostRules || []), ...rules];\n" +
		"} catch (_) {\n" +
		"  throw new Error('Cannot load global host rules Secret');\n" +
		"}\n"
}
