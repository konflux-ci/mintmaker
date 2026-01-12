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
	"regexp"
	"strings"
)

// SanitizeLabelValue sanitizes a string to be valid for Kubernetes label values.
// It handles common restrictions like maximum length (63 chars) and disallowed characters.
func NormalizeLabelValue(value string) string {
	// A valid label must be an empty string or consist of alphanumeric
	// characters, '-', '_' or '.', and must start and end with an
	// alphanumeric character

	// Replace forward slashes with underscores (they're not allowed)
	sanitized := strings.ReplaceAll(value, "/", "_")

	// Ensure the length is within limits of 63
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
	}

	// Strip leading non-alphanumeric characters
	sanitized = regexp.MustCompile(`^[^A-Za-z0-9]+`).ReplaceAllString(sanitized, "")

	// Strip trailing non-alphanumeric characters
	sanitized = regexp.MustCompile(`[^A-Za-z0-9]+$`).ReplaceAllString(sanitized, "")

	// Further validation to ensure the sanitized string follows the exact pattern
	// This would replace any sequences of invalid characters in the middle with a underscore
	sanitized = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(sanitized, "_")

	return sanitized
}
