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

package base

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func renovateConfigMap(goproxy string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-config", Namespace: "mintmaker"},
		Data: map[string]string{
			"renovate.json":    `{"dryRun": "full"}`,
			"self_hosted.json": `{"GOPROXY": "` + goproxy + `"}`,
		},
	}
}

// TestGetRenovateBaseConfigReflectsConfigMapChanges verifies that a change to the
// renovate-config ConfigMap is picked up on the next call, without requiring a
// controller restart (issue #544). It fails if GetRenovateBaseConfig caches the
// config for the process lifetime.
func TestGetRenovateBaseConfigReflectsConfigMapChanges(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(renovateConfigMap("https://proxy.internal/old")).
		Build()

	c := &BaseComponent{}
	ctx := context.Background()
	key := types.NamespacedName{Namespace: "mintmaker", Name: "renovate-config"}

	config, err := c.GetRenovateBaseConfig(ctx, cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := config["GOPROXY"]; got != "https://proxy.internal/old" {
		t.Fatalf("expected initial GOPROXY %q, got %v", "https://proxy.internal/old", got)
	}

	// Simulate an operator editing the ConfigMap (e.g. changing GOPROXY).
	live := &corev1.ConfigMap{}
	if err := cl.Get(ctx, key, live); err != nil {
		t.Fatalf("failed to fetch configmap: %v", err)
	}
	live.Data["self_hosted.json"] = `{"GOPROXY": "https://proxy.internal/new"}`
	if err := cl.Update(ctx, live); err != nil {
		t.Fatalf("failed to update configmap: %v", err)
	}

	config, err = c.GetRenovateBaseConfig(ctx, cl)
	if err != nil {
		t.Fatalf("unexpected error on re-read: %v", err)
	}
	if got := config["GOPROXY"]; got != "https://proxy.internal/new" {
		t.Fatalf("GetRenovateBaseConfig returned stale config after ConfigMap change: GOPROXY=%v, want %q", got, "https://proxy.internal/new")
	}
}
