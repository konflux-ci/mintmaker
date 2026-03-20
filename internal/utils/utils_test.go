package utils

import (
	"testing"
)

func TestGetGitPlatform(t *testing.T) {
	tests := []struct {
		name        string
		giturl      string
		expected    string
		expectError bool
	}{
		{
			name:     "GitHub HTTPS URL",
			giturl:   "https://github.com/owner/repo",
			expected: "github",
		},
		{
			name:     "GitLab HTTPS URL",
			giturl:   "https://gitlab.com/owner/repo",
			expected: "gitlab",
		},
		{
			name:     "Forgejo HTTPS URL",
			giturl:   "https://forge.example.com/owner/repo",
			expected: "forgejo",
		},
		{
			name:     "GitHub SSH URL",
			giturl:   "git@github.com:owner/repo.git",
			expected: "github",
		},
		{
			name:     "GitLab SSH URL",
			giturl:   "git@gitlab.com:owner/repo.git",
			expected: "gitlab",
		},
		{
			name:        "unsupported platform",
			giturl:      "https://bitbucket.org/owner/repo",
			expectError: true,
		},
		{
			name:        "empty URL",
			giturl:      "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := GetGitPlatform(tc.giturl)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetGitHost(t *testing.T) {
	tests := []struct {
		name        string
		giturl      string
		expected    string
		expectError bool
	}{
		{
			name:     "HTTPS URL",
			giturl:   "https://github.com/owner/repo",
			expected: "github.com",
		},
		{
			name:     "HTTPS URL with port",
			giturl:   "https://gitlab.example.com:8443/owner/repo",
			expected: "gitlab.example.com",
		},
		{
			name:     "SSH URL",
			giturl:   "git@github.com:owner/repo.git",
			expected: "github.com",
		},
		{
			name:     "SSH URL with custom user",
			giturl:   "user@gitlab.com:owner/repo.git",
			expected: "gitlab.com",
		},
		{
			name:     "plain HTTP URL",
			giturl:   "http://forge.example.com/owner/repo",
			expected: "forge.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := GetGitHost(tc.giturl)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetGitPath(t *testing.T) {
	tests := []struct {
		name        string
		giturl      string
		expected    string
		expectError bool
	}{
		{
			name:     "HTTPS URL",
			giturl:   "https://github.com/owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "HTTPS URL with .git suffix",
			giturl:   "https://github.com/owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "HTTPS URL with trailing slash",
			giturl:   "https://github.com/owner/repo/",
			expected: "owner/repo",
		},
		{
			name:     "SSH URL",
			giturl:   "git@github.com:owner/repo.git",
			expected: "owner/repo",
		},
		{
			name:     "SSH URL without .git suffix",
			giturl:   "git@github.com:owner/repo",
			expected: "owner/repo",
		},
		{
			name:     "nested path",
			giturl:   "https://gitlab.com/group/subgroup/repo",
			expected: "group/subgroup/repo",
		},
		{
			name:     "SSH nested path",
			giturl:   "git@gitlab.com:group/subgroup/repo.git",
			expected: "group/subgroup/repo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := GetGitPath(tc.giturl)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
