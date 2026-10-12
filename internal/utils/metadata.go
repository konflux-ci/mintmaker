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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetLabel returns the label value for newKey if set, otherwise oldKey.
func GetLabel(obj metav1.Object, newKey, oldKey string) string {
	labels := obj.GetLabels()
	if labels == nil {
		return ""
	}
	if v := labels[newKey]; v != "" {
		return v
	}
	return labels[oldKey]
}

// GetAnnotation returns the annotation value for newKey if set, otherwise oldKey.
func GetAnnotation(obj metav1.Object, newKey, oldKey string) string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return ""
	}
	if v := annotations[newKey]; v != "" {
		return v
	}
	return annotations[oldKey]
}

// HasAnnotation reports whether newKey or oldKey is present (even if empty).
func HasAnnotation(obj metav1.Object, newKey, oldKey string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	if _, ok := annotations[newKey]; ok {
		return true
	}
	_, ok := annotations[oldKey]
	return ok
}
