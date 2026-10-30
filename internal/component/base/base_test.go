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

package base

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRenovateBaseConfigIsIsolatedPerRun(t *testing.T) {
	renovateBaseConfigMutex.Lock()
	renovateBaseConfig = nil
	renovateBaseConfigMutex.Unlock()
	t.Cleanup(func() {
		renovateBaseConfigMutex.Lock()
		renovateBaseConfig = nil
		renovateBaseConfigMutex.Unlock()
	})

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-config", Namespace: "mintmaker"},
		Data: map[string]string{
			"renovate.json":    `{"hostRules":[{"hostType":"docker","matchHost":"base.example"}],"nested":{"value":"base"}}`,
			"self_hosted.json": `{}`,
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
	component := &BaseComponent{}
	first, err := component.GetRenovateBaseConfig(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	first["hostRules"].([]interface{})[0].(map[string]interface{})["matchHost"] = "changed.example"
	first["nested"].(map[string]interface{})["value"] = "changed"
	first["repositories"] = []string{"first"}

	second, err := component.GetRenovateBaseConfig(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if got := second["hostRules"].([]interface{})[0].(map[string]interface{})["matchHost"]; got != "base.example" {
		t.Errorf("cached host rule was changed: %v", got)
	}
	if got := second["nested"].(map[string]interface{})["value"]; got != "base" {
		t.Errorf("cached nested value was changed: %v", got)
	}
	if _, exists := second["repositories"]; exists {
		t.Error("repository setting leaked between runs")
	}
}
