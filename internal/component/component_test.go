package component

import (
	"testing"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
