package component

import (
	"testing"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetGitURL(t *testing.T) {
	tests := []struct {
		name           string
		comp           *appstudiov1alpha1.Component
		expectedURL    string
		expectedOldCRD bool
		expectError    bool
	}{
		{
			name: "old CRD with GitSource",
			comp: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{
								URL: "https://github.com/owner/repo",
							},
						},
					},
				},
			},
			expectedURL:    "https://github.com/owner/repo",
			expectedOldCRD: true,
		},
		{
			name: "new CRD with GitURL",
			comp: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitURL: "https://github.com/owner/repo2",
						},
					},
				},
			},
			expectedURL:    "https://github.com/owner/repo2",
			expectedOldCRD: false,
		},
		{
			name: "no git source returns error",
			comp: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec:       appstudiov1alpha1.ComponentSpec{},
			},
			expectError: true,
		},
		{
			name: "old CRD with empty GitSource URL returns error",
			comp: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{URL: ""},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "new CRD with empty GitURL returns error",
			comp: &appstudiov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "test-comp"},
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitURL: "",
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, oldCRD, err := GetGitURL(tc.comp)
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
			if oldCRD != tc.expectedOldCRD {
				t.Errorf("oldCRD: got %v, want %v", oldCRD, tc.expectedOldCRD)
			}
		})
	}
}

func TestGetVersions(t *testing.T) {
	tests := []struct {
		name     string
		comp     *appstudiov1alpha1.Component
		expected []string
	}{
		{
			name: "old CRD with revision",
			comp: &appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{Revision: "main"},
						},
					},
				},
			},
			expected: []string{"main"},
		},
		{
			name: "new CRD with versions",
			comp: &appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							Versions: []appstudiov1alpha1.ComponentVersion{
								{Name: "main-version", Revision: "main"},
								{Name: "release-1.0-version", Revision: "release-1.0"},
							},
						},
					},
				},
			},
			expected: []string{"main", "release-1.0"},
		},
		{
			name: "no versions returns empty slice",
			comp: &appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{},
			},
			expected: []string{},
		},
		{
			name: "old CRD with empty revision returns empty slice",
			comp: &appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{Revision: ""},
						},
					},
				},
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
