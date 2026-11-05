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

package gitlab

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/konflux-ci/mintmaker/internal/component/base"
	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

func newTestComponent(cl client.Client, host, repository string) *Component {
	return &Component{
		BaseComponent: base.BaseComponent{
			Namespace:  "test-ns",
			Host:       host,
			Repository: repository,
		},
		client: cl,
		ctx:    context.Background(),
	}
}

func scmSecret(name, host string, labels map[string]string, annotations map[string]string) *corev1.Secret {
	if labels == nil {
		labels = map[string]string{
			mmconst.CredentialsLabel: "scm",
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
		Type: corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte("user"),
			corev1.BasicAuthPasswordKey: []byte("token-" + name),
		},
	}
}

func TestLookupSecret(t *testing.T) {
	const host = "gitlab.com"

	tests := []struct {
		name           string
		repository     string
		objects        []client.Object
		expectError    bool
		expectedSecret string
	}{
		{
			name:        "no secrets",
			repository:  "org/repo",
			expectError: true,
		},
		{
			name:       "ignores non-BasicAuth secrets",
			repository: "org/repo",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "opaque",
						Namespace: "test-ns",
						Labels: map[string]string{
							mmconst.CredentialsLabel: "scm",
							mmconst.SCMHostLabel:     host,
						},
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{"token": []byte("x")},
				},
			},
			expectError: true,
		},
		{
			name:       "host-only secret without repository annotation",
			repository: "org/repo",
			objects: []client.Object{
				scmSecret("host-secret", host, nil, nil),
			},
			expectedSecret: "host-secret",
		},
		{
			name:       "exact repository match",
			repository: "org/repo",
			objects: []client.Object{
				scmSecret("host-secret", host, nil, nil),
				scmSecret("exact-secret", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/repo",
				}),
			},
			expectedSecret: "exact-secret",
		},
		{
			name:       "repository mismatch and no host-only secret",
			repository: "org/repo",
			objects: []client.Object{
				scmSecret("other-repo", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "other/repo",
				}),
			},
			expectError: true,
		},
		{
			name:       "best wildcard match",
			repository: "org/repo/foo",
			objects: []client.Object{
				scmSecret("broad", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/*",
				}),
				scmSecret("narrow", host, nil, map[string]string{
					mmconst.SCMRepositoryAnnotation: "org/repo/*",
				}),
			},
			expectedSecret: "narrow",
		},
		{
			name:       "legacy labels",
			repository: "org/repo",
			objects: []client.Object{
				scmSecret("legacy-secret", host, map[string]string{
					mmconst.CredentialsLabelLegacy: "scm",
					mmconst.SCMHostLabelLegacy:     host,
				}, nil),
			},
			expectedSecret: "legacy-secret",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(tc.objects...).Build()
			secret, err := newTestComponent(cl, host, tc.repository).lookupSecret()
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if secret.Name != tc.expectedSecret {
				t.Errorf("secret: got %q, want %q", secret.Name, tc.expectedSecret)
			}
		})
	}
}
