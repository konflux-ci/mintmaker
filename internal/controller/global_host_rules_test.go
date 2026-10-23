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
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/konflux-ci/mintmaker/internal/component/mocks"
)

func TestValidateGlobalHostRules(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"multiple host types and credential forms", `[{"hostType":"pypi","matchHost":"https://index.example.com/simple/","username":"user","password":"private-password"},{"hostType":"helm","matchHost":"charts.example.com","token":"private-token"}]`, false},
		{"empty array", `[]`, false},
		{"new host type", `[{"hostType":"future-datasource","matchHost":"future.example.com","token":"private-token"}]`, false},
		{"missing host type", `[{"matchHost":"index.example.com","token":"private-token"}]`, true},
		{"missing host", `[{"hostType":"pypi","token":"private-token"}]`, true},
		{"host only path", `[{"hostType":"pypi","matchHost":"index.example.com/simple","token":"private-token"}]`, true},
		{"host with URL credentials", `[{"hostType":"pypi","matchHost":"https://user:pass@index.example.com/simple","token":"private-token"}]`, true},
		{"wildcard host", `[{"hostType":"pypi","matchHost":"*.example.com","token":"private-token"}]`, true},
		{"incomplete basic auth", `[{"hostType":"pypi","matchHost":"index.example.com","username":"user"}]`, true},
		{"ambiguous credentials", `[{"hostType":"pypi","matchHost":"index.example.com","token":"private-token","username":"user","password":"private-password"}]`, true},
		{"unknown field", `[{"hostType":"pypi","matchHost":"index.example.com","token":"private-token","headers":{"Authorization":"private-password"}}]`, true},
		{"mixed case matchHost could widen credential scope", `[{"hostType":"pypi","MatchHost":"trusted.example.com","token":"private-token"}]`, true},
		{"mixed case hostType", `[{"HostType":"pypi","matchHost":"trusted.example.com","token":"private-token"}]`, true},
		{"mixed case token", `[{"hostType":"pypi","matchHost":"trusted.example.com","Token":"private-token"}]`, true},
		{"mixed case username", `[{"hostType":"pypi","matchHost":"trusted.example.com","UserName":"user","password":"private-password"}]`, true},
		{"mixed case password", `[{"hostType":"pypi","matchHost":"trusted.example.com","username":"user","Password":"private-password"}]`, true},
		{"exact and mixed case matchHost", `[{"hostType":"pypi","matchHost":"trusted.example.com","MatchHost":"other.example.com","token":"private-token"}]`, true},
		{"malformed JSON", `[{"hostType":"pypi","token":"private-token"`, true},
		{"not an array", `{"hostType":"pypi"}`, true},
		{"extra content", `[] []`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGlobalHostRules([]byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateGlobalHostRules() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && (strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "private-password")) {
				t.Errorf("error reveals a credential: %v", err)
			}
		})
	}
}

func TestValidateHostRulesSecret(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rules", Namespace: "mintmaker"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{hostRulesSecretKey: []byte(`[]`)},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	reconciler := &DependencyUpdateCheckReconciler{Client: cl}
	if err := reconciler.validateHostRulesSecret(ctx, secret.Name); err != nil {
		t.Fatal(err)
	}
	delete(secret.Data, hostRulesSecretKey)
	if err := cl.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.validateHostRulesSecret(ctx, secret.Name); err == nil || !strings.Contains(err.Error(), hostRulesSecretKey) {
		t.Fatalf("missing key: got %v", err)
	}
	secret.Type = corev1.SecretTypeBasicAuth
	if err := cl.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.validateHostRulesSecret(ctx, secret.Name); err == nil || !strings.Contains(err.Error(), "Opaque") {
		t.Fatalf("wrong type: got %v", err)
	}
}

func TestGlobalHostRulesSecretAndPipelineRun(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := tektonv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-host-rules", Namespace: "mintmaker"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{hostRulesSecretKey: []byte(`[{"hostType":"pypi","matchHost":"https://index.example.com/simple/","username":"user","password":"global-password"},{"hostType":"helm","matchHost":"charts.example.com","token":"global-token"}]`)},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	reconciler := &DependencyUpdateCheckReconciler{Client: cl, Scheme: scheme}
	comp := mocks.NewMockGitComponent(t)
	comp.EXPECT().GetPlatform().Return("gitlab").Maybe()
	comp.EXPECT().GetToken().Return("repository-token", nil).Maybe()
	comp.EXPECT().GetNamespace().Return("tenant").Maybe()
	comp.EXPECT().GetName().Return("component").Maybe()
	comp.EXPECT().GetApplication().Return("application").Maybe()
	comp.EXPECT().GetHost().Return("gitlab.example.com").Maybe()
	comp.EXPECT().GetRepository().Return("org/repo").Maybe()
	comp.EXPECT().GetRenovateConfig(mock.Anything, "main").Return(`{"hostRules":[{"hostType":"docker","matchHost":"registry.component.example","username":"component","password":"component-password"}]}`, nil).Maybe()
	comp.EXPECT().GetRPMActivationKey(ctx, cl).Return("", "", errors.New("no RPM key")).Maybe()

	if _, err := reconciler.createPipelineRun(ctx, "missing-global-secret", comp, "main", "", "missing-secret"); err == nil || !strings.Contains(err.Error(), "missing-secret") {
		t.Fatalf("missing configured Secret: got %v", err)
	}
	if err := reconciler.validateHostRulesSecret(ctx, secret.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.createPipelineRun(ctx, "run-one", comp, "main", "", secret.Name); err != nil {
		t.Fatal(err)
	}

	configMap := &corev1.ConfigMap{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "run-one", Namespace: "mintmaker"}, configMap); err != nil {
		t.Fatal(err)
	}
	configJS := configMap.Data["config.js"]
	if !strings.Contains(configJS, "component-password") || !strings.Contains(configJS, hostRulesSecretPath) {
		t.Error("component rule or Secret loader is missing from the generated config")
	}
	if strings.Contains(configJS, "global-password") || strings.Contains(configJS, "global-token") {
		t.Error("global credentials were written to the ConfigMap")
	}
	if !strings.Contains(configJS, "...(module.exports.hostRules || [])") {
		t.Error("global rules are not appended to component rules")
	}
	pr := &tektonv1.PipelineRun{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "run-one", Namespace: "mintmaker"}, pr); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(pr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "global-password") || strings.Contains(string(serialized), "global-token") {
		t.Error("global credentials were written to the PipelineRun")
	}
	task := pr.Spec.PipelineSpec.Tasks[0].TaskSpec
	foundVolume := false
	for _, volume := range task.Volumes {
		if volume.Secret != nil && volume.Secret.SecretName == secret.Name {
			foundVolume = len(volume.Secret.Items) == 1 && volume.Secret.Items[0].Key == hostRulesSecretKey
		}
	}
	if !foundVolume {
		t.Error("host rules Secret was not mounted in the PipelineRun")
	}
	foundMount := false
	for _, step := range task.Steps {
		for _, mount := range step.VolumeMounts {
			if mount.MountPath == hostRulesMountPath && (step.Name != "renovate" || !mount.ReadOnly) {
				t.Errorf("host rules mounted on unexpected step or without read-only flag: %s", step.Name)
			}
			if mount.MountPath == hostRulesMountPath && step.Name == "renovate" && mount.ReadOnly {
				foundMount = true
			}
		}
	}
	if !foundMount {
		t.Error("host rules Secret is not mounted on the Renovate step")
	}

	secret.Data[hostRulesSecretKey] = []byte(`[{"hostType":"future-datasource","matchHost":"future.example.com","token":"rotated-token"}]`)
	if err := cl.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.createPipelineRun(ctx, "run-two", comp, "main", "", secret.Name); err != nil {
		t.Fatal(err)
	}
	secret.Data[hostRulesSecretKey] = []byte(`[{"hostType":"pypi","matchHost":"index.example.com","token":"bad-token","password":"bad-password"}]`)
	if err := cl.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.createPipelineRun(ctx, "run-three", comp, "main", "", secret.Name); err == nil || strings.Contains(err.Error(), "bad-token") || strings.Contains(err.Error(), "bad-password") {
		t.Fatalf("invalid Secret error is missing or reveals credentials: %v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "run-three", Namespace: "mintmaker"}, &corev1.ConfigMap{}); err == nil {
		t.Error("invalid Secret created a per-run ConfigMap")
	}
	secret.Data[hostRulesSecretKey] = []byte(`[{"hostType":"pypi","MatchHost":"trusted.example.com","token":"private-token"}]`)
	if err := cl.Update(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.createPipelineRun(ctx, "run-four", comp, "main", "", secret.Name); err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("mis-cased matchHost error is missing or reveals credentials: %v", err)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "run-four", Namespace: "mintmaker"}, &corev1.ConfigMap{}); err == nil {
		t.Error("mis-cased matchHost created a per-run ConfigMap")
	}
	if got := renovateConfigJS(`{"hostRules":[]}`, false); got != `module.exports = {"hostRules":[]}` {
		t.Errorf("unconfigured behavior changed: %s", got)
	}
}
