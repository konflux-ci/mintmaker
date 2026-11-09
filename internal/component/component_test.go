package component

import (
	"context"
	"testing"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

func newTestComponent(name, gitURL string, versions []konfluxv1alpha1.ComponentVersion) *konfluxv1alpha1.Component {
	return &konfluxv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
			Annotations: map[string]string{
				mmconst.MintMakerConvertedApplicationAnnotation: "test-app",
			},
		},
		Spec: konfluxv1alpha1.ComponentSpec{
			Source: konfluxv1alpha1.ComponentSource{
				GitURL:   gitURL,
				Versions: versions,
			},
		},
	}
}

func TestNewGitComponent(t *testing.T) {
	ctx := context.Background()
	githubAppSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipelines-as-code-secret",
			Namespace: "mintmaker",
		},
		Data: map[string][]byte{
			"github-application-id": []byte("12345"),
			"github-private-key":    []byte("fake-private-key"),
		},
	}
	tests := []struct {
		name           string
		comp           *konfluxv1alpha1.Component
		objects        []client.Object
		expectError    bool
		wantPlatform   string
		wantHost       string
		wantRepository string
	}{
		{
			name: "empty GitURL returns error",
			comp: &konfluxv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-url"},
			},
			expectError: true,
		},
		{
			name:        "unsupported platform returns error",
			comp:        newTestComponent("bitbucket-comp", "https://bitbucket.org/owner/repo.git", nil),
			expectError: true,
		},
		{
			name: "github without app secret returns error",
			comp: newTestComponent("gh-comp", "https://github.com/owner/repo.git", []konfluxv1alpha1.ComponentVersion{
				{Name: "default", Revision: "main"},
			}),
			expectError: true,
		},
		{
			name: "github component",
			comp: newTestComponent("gh-comp", "https://github.com/owner/repo.git", []konfluxv1alpha1.ComponentVersion{
				{Name: "default", Revision: "main"},
			}),
			objects:        []client.Object{githubAppSecret},
			wantPlatform:   "github",
			wantHost:       "github.com",
			wantRepository: "owner/repo",
		},
		{
			name: "gitlab component",
			comp: newTestComponent("gl-comp", "https://gitlab.com/owner/repo.git", []konfluxv1alpha1.ComponentVersion{
				{Name: "default", Revision: "main"},
			}),
			wantPlatform:   "gitlab",
			wantHost:       "gitlab.com",
			wantRepository: "owner/repo",
		},
		{
			name: "forgejo component",
			comp: newTestComponent("fj-comp", "https://forgejo.example.com/owner/repo.git", []konfluxv1alpha1.ComponentVersion{
				{Name: "default", Revision: "main"},
			}),
			wantPlatform:   "forgejo",
			wantHost:       "forgejo.example.com",
			wantRepository: "owner/repo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithObjects(tc.objects...).Build()
			gitComp, err := NewGitComponent(ctx, tc.comp, cl)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gitComp.GetName() != tc.comp.Name {
				t.Errorf("Name: got %q, want %q", gitComp.GetName(), tc.comp.Name)
			}
			if gitComp.GetNamespace() != tc.comp.Namespace {
				t.Errorf("Namespace: got %q, want %q", gitComp.GetNamespace(), tc.comp.Namespace)
			}
			if gitComp.GetApplication() != "test-app" {
				t.Errorf("Application: got %q, want %q", gitComp.GetApplication(), "test-app")
			}
			if gitComp.GetPlatform() != tc.wantPlatform {
				t.Errorf("Platform: got %q, want %q", gitComp.GetPlatform(), tc.wantPlatform)
			}
			if gitComp.GetHost() != tc.wantHost {
				t.Errorf("Host: got %q, want %q", gitComp.GetHost(), tc.wantHost)
			}
			if gitComp.GetRepository() != tc.wantRepository {
				t.Errorf("Repository: got %q, want %q", gitComp.GetRepository(), tc.wantRepository)
			}
		})
	}
}

func TestGetGitURL(t *testing.T) {
	tests := []struct {
		name        string
		comp        *konfluxv1alpha1.Component
		expectedURL string
		expectError bool
	}{
		{
			name: "with GitURL",
			comp: &konfluxv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec: konfluxv1alpha1.ComponentSpec{
					Source: konfluxv1alpha1.ComponentSource{GitURL: "https://github.com/owner/repo"},
				},
			},
			expectedURL: "https://github.com/owner/repo",
		},
		{
			name: "empty GitURL returns error",
			comp: &konfluxv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec:       konfluxv1alpha1.ComponentSpec{},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := GetGitURL(tc.comp)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if url != tc.expectedURL {
				t.Errorf("URL: got %q, want %q", url, tc.expectedURL)
			}
		})
	}
}

func TestGetVersions(t *testing.T) {
	tests := []struct {
		name     string
		comp     *konfluxv1alpha1.Component
		expected []string
	}{
		{
			name: "with versions",
			comp: &konfluxv1alpha1.Component{
				Spec: konfluxv1alpha1.ComponentSpec{
					Source: konfluxv1alpha1.ComponentSource{Versions: []konfluxv1alpha1.ComponentVersion{
						{Name: "main-version", Revision: "main"},
						{Name: "release-1.0-version", Revision: "release-1.0"},
					}},
				},
			},
			expected: []string{"main", "release-1.0"},
		},
		{
			name: "no versions returns empty slice",
			comp: &konfluxv1alpha1.Component{
				Spec: konfluxv1alpha1.ComponentSpec{},
			},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetVersions(tc.comp)
			if len(result) != len(tc.expected) {
				t.Fatalf("got %d versions %v, want %d versions %v", len(result), result, len(tc.expected), tc.expected)
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("version[%d]: got %q, want %q", i, v, tc.expected[i])
				}
			}
		})
	}
}
