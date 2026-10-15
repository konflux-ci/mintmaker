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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

func rpmSecret(name, host string, labels map[string]string, annotations map[string]string, key, org string) *corev1.Secret {
	if labels == nil {
		labels = map[string]string{
			mmconst.CredentialsLabel: "rpm",
			mmconst.SCMHostLabel:     host,
		}
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "test-ns",
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"activationkey": []byte(key),
			"org":           []byte(org),
		},
	}
}

func TestGetRPMActivationKey(t *testing.T) {
	ctx := context.Background()
	const host = "gitlab.com"

	tests := []struct {
		name        string
		repository  string
		objects     []client.Object
		expectError bool
		wantKey     string
		wantOrg     string
	}{
		{
			name:        "no secrets",
			repository:  "org/repo",
			expectError: true,
		},
		{
			name:       "falls back to default secret",
			repository: "org/repo",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "activation-key", Namespace: "test-ns"},
					Type:       corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"activationkey": []byte("default-key"),
						"org":           []byte("default-org"),
					},
				},
			},
			wantKey: "default-key",
			wantOrg: "default-org",
		},
		{
			name:       "default secret missing fields",
			repository: "org/repo",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "activation-key", Namespace: "test-ns"},
					Type:       corev1.SecretTypeOpaque,
					Data:       map[string][]byte{"activationkey": []byte("only-key")},
				},
			},
			expectError: true,
		},
		{
			name:       "ignores non-Opaque secrets",
			repository: "org/repo",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-auth",
						Namespace: "test-ns",
						Labels: map[string]string{
							mmconst.CredentialsLabel: "rpm",
							mmconst.SCMHostLabel:     host,
						},
					},
					Type: corev1.SecretTypeBasicAuth,
				},
			},
			expectError: true,
		},
		{
			name:       "host-only secret without repository annotation",
			repository: "org/repo",
			objects: []client.Object{
				rpmSecret("rpm-host", host, nil, nil, "host-key", "host-org"),
			},
			wantKey: "host-key",
			wantOrg: "host-org",
		},
		{
			name:       "exact repository match",
			repository: "org/repo",
			objects: []client.Object{
				rpmSecret("rpm-host", host, nil, nil, "host-key", "host-org"),
				rpmSecret("rpm-exact", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/repo",
				}, "exact-key", "exact-org"),
			},
			wantKey: "exact-key",
			wantOrg: "exact-org",
		},
		{
			name:       "best wildcard match",
			repository: "org/repo/foo",
			objects: []client.Object{
				rpmSecret("broad", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/*",
				}, "broad-key", "broad-org"),
				rpmSecret("narrow", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/repo/*",
				}, "narrow-key", "narrow-org"),
			},
			wantKey: "narrow-key",
			wantOrg: "narrow-org",
		},
		{
			name:       "legacy labels",
			repository: "org/repo",
			objects: []client.Object{
				rpmSecret("legacy-rpm", host, map[string]string{
					mmconst.CredentialsLabelLegacy: "rpm",
					mmconst.SCMHostLabelLegacy:     host,
				}, nil, "legacy-key", "legacy-org"),
			},
			wantKey: "legacy-key",
			wantOrg: "legacy-org",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(tc.objects...).Build()
			c := &BaseComponent{Namespace: "test-ns", Host: host, Repository: tc.repository}

			key, org, err := c.GetRPMActivationKey(ctx, cl)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tc.wantKey || org != tc.wantOrg {
				t.Errorf("got (%q, %q), want (%q, %q)", key, org, tc.wantKey, tc.wantOrg)
			}
		})
	}
}
