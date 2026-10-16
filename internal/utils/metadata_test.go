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

package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetLabel(t *testing.T) {
	tests := []struct {
		name     string
		obj      metav1.Object
		newKey   string
		oldKey   string
		expected string
	}{
		{
			name:     "empty labels",
			obj:      &metav1.ObjectMeta{},
			newKey:   "new-label",
			oldKey:   "old-label",
			expected: "",
		},
		{
			name: "new and old label set",
			obj: &metav1.ObjectMeta{
				Labels: map[string]string{
					"new-label": "new-label-value",
					"old-label": "old-label-value",
				},
			},
			newKey:   "new-label",
			oldKey:   "old-label",
			expected: "new-label-value",
		},
		{
			name: "old label set only",
			obj: &metav1.ObjectMeta{
				Labels: map[string]string{
					"old-label": "old-label-value",
				},
			},
			newKey:   "new-label",
			oldKey:   "old-label",
			expected: "old-label-value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetLabel(tc.obj, tc.newKey, tc.oldKey)
			if got != tc.expected {
				t.Errorf("GetLabel(%s, %s, %s) = %s, want %s", tc.name, tc.newKey, tc.oldKey, got, tc.expected)
			}
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		obj      metav1.Object
		newKey   string
		oldKey   string
		expected string
	}{
		{
			name:     "empty annotations",
			obj:      &metav1.ObjectMeta{},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: "",
		},
		{
			name: "new and old annotation set",
			obj: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"new-annotation": "new-annotation-value",
					"old-annotation": "old-annotation-value",
				},
			},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: "new-annotation-value",
		},
		{
			name: "old annotation set only",
			obj: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"old-annotation": "old-annotation-value",
				},
			},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: "old-annotation-value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetAnnotation(tc.obj, tc.newKey, tc.oldKey)
			if got != tc.expected {
				t.Errorf("GetAnnotation(%s, %s, %s) = %s, want %s", tc.name, tc.newKey, tc.oldKey, got, tc.expected)
			}
		})
	}
}

func TestHasAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		obj      metav1.Object
		newKey   string
		oldKey   string
		expected bool
	}{
		{
			name:     "empty annotations",
			obj:      &metav1.ObjectMeta{},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: false,
		},
		{
			name: "new and old annotation set",
			obj: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"new-annotation": "new-annotation-value",
					"old-annotation": "old-annotation-value",
				},
			},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: true,
		},
		{
			name: "old annotation set only",
			obj: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"old-annotation": "old-annotation-value",
				},
			},
			newKey:   "new-annotation",
			oldKey:   "old-annotation",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HasAnnotation(tc.obj, tc.newKey, tc.oldKey)
			if got != tc.expected {
				t.Errorf("HasAnnotation(%s, %s, %s) = %t, want %t", tc.name, tc.newKey, tc.oldKey, got, tc.expected)
			}
		})
	}
}
