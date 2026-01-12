// Copyright 2025 Red Hat, Inc.
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

package utils

import (
	"testing"
)

func TestNormalizeLabelValue(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		description string
	}{
		{
			name:        "Basic repository path",
			input:       "owner/repo-name",
			expected:    "owner_repo-name",
			description: "Should replace slashes with hyphens",
		},
		{
			name:        "With underscores",
			input:       "owner_name/repo_name",
			expected:    "owner_name_repo_name",
			description: "Should keep underscores but replace slashes",
		},
		{
			name:        "With dots",
			input:       "owner.name/repo.name",
			expected:    "owner.name_repo.name",
			description: "Should keep dots but replace slashes",
		},
		{
			name:        "With special characters",
			input:       "owner$name/repo@name",
			expected:    "owner_name_repo_name",
			description: "Should remove invalid special characters",
		},
		{
			name:        "Leading special characters",
			input:       "-_./owner/repo",
			expected:    "owner_repo",
			description: "Should strip leading non-alphanumeric characters",
		},
		{
			name:        "Trailing special characters",
			input:       "owner/repo-_./",
			expected:    "owner_repo",
			description: "Should strip trailing non-alphanumeric characters",
		},
		{
			name:        "Too long",
			input:       "very-long-owner-name/extremely-long-repository-name-that-exceeds-the-maximum-length-for-kubernetes",
			expected:    "very-long-owner-name_extremely-long-repository-name-that-exceed",
			description: "Should truncate to max length of 63",
		},
		{
			name:        "Empty string",
			input:       "",
			expected:    "",
			description: "Should keep empty strings",
		},
		{
			name:        "Only special characters",
			input:       "-_./&^%$#@!",
			expected:    "",
			description: "Should use default when no valid characters remain",
		},
		{
			name:        "Mixed valid and invalid",
			input:       "valid-123_name.example/with&invalid^chars",
			expected:    "valid-123_name.example_with_invalid_chars",
			description: "Should preserve valid characters and replace invalid ones",
		},
		{
			name:        "With consecutive invalid characters",
			input:       "name-with!!!multiple@@@invalid###chars",
			expected:    "name-with_multiple_invalid_chars",
			description: "Should replace sequences of invalid chars with single hyphen",
		},
		{
			name:        "With exactly 63 characters",
			input:       "exactly-sixty-three-chars-long-label-value-for-kubernetes-label",
			expected:    "exactly-sixty-three-chars-long-label-value-for-kubernetes-label",
			description: "Should keep string unchanged if exactly at max length",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeLabelValue(tc.input)
			if result != tc.expected {
				t.Errorf("NormalizeLabelValue(%q) = %q, expected %q\n%s",
					tc.input, result, tc.expected, tc.description)
			}

			// Additional validation: check that result adheres to Kubernetes label value format
			if !isValidKubernetesLabelValue(result) {
				t.Errorf("Result %q is not a valid Kubernetes label value", result)
			}

		})
	}
}

// Helper function to validate a Kubernetes label value based on the regex:
// '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?'
func isValidKubernetesLabelValue(value string) bool {
	// Empty string is valid
	if value == "" {
		return true
	}

	// Must start with alphanumeric
	if len(value) > 0 && !isAlphanumeric(rune(value[0])) {
		return false
	}

	// Must end with alphanumeric
	if len(value) > 0 && !isAlphanumeric(rune(value[len(value)-1])) {
		return false
	}

	// Middle characters must be alphanumeric, '-', '_', or '.'
	for _, r := range value[1 : len(value)-1] {
		if !isAlphanumeric(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}

	return len(value) <= 63
}

func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
