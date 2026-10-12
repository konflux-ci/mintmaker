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

package component

import (
	"testing"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
)

func TestToKonfluxComponent(t *testing.T) {
	t.Run("old model with revision", func(t *testing.T) {
		src := &appstudiov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "c",
				Namespace: "ns",
				Annotations: map[string]string{
					mmconst.MintMakerDisabledAnnotationNameLegacy: "true",
				},
			},
			Spec: appstudiov1alpha1.ComponentSpec{
				Application: "my-app",
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitSource: &appstudiov1alpha1.GitSource{
							URL:      "https://github.com/owner/repo",
							Revision: "main",
						},
					},
				},
			},
		}
		dst := ToKonfluxComponent(src)
		if dst.Name != "c" || dst.Namespace != "ns" {
			t.Errorf("Name/Namespace = %s/%s, want c/ns", dst.Name, dst.Namespace)
		}
		if dst.Spec.Source.GitURL != "https://github.com/owner/repo" {
			t.Errorf("GitURL = %q, want %q", dst.Spec.Source.GitURL, "https://github.com/owner/repo")
		}
		if len(dst.Spec.Source.Versions) != 1 {
			t.Fatalf("Versions len = %d, want 1", len(dst.Spec.Source.Versions))
		}
		if dst.Spec.Source.Versions[0].Name != "main" || dst.Spec.Source.Versions[0].Revision != "main" {
			t.Errorf("Versions[0] = %+v, want Name/Revision main", dst.Spec.Source.Versions[0])
		}
		if dst.Annotations[mmconst.MintMakerConvertedApplicationAnnotation] != "my-app" {
			t.Errorf("converted-application = %q, want my-app", dst.Annotations[mmconst.MintMakerConvertedApplicationAnnotation])
		}
		if dst.Annotations[mmconst.MintMakerOldCRDAnnotation] != "true" {
			t.Errorf("old-crd = %q, want true", dst.Annotations[mmconst.MintMakerOldCRDAnnotation])
		}
		if dst.Annotations[mmconst.MintMakerDisabledAnnotationNameLegacy] != "true" {
			t.Errorf("disabled annotation not cloned")
		}
	})

	t.Run("old model without revision", func(t *testing.T) {
		src := &appstudiov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
			Spec: appstudiov1alpha1.ComponentSpec{
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitSource: &appstudiov1alpha1.GitSource{
							URL: "https://github.com/owner/repo",
						},
					},
				},
			},
		}
		dst := ToKonfluxComponent(src)
		if dst.Spec.Source.GitURL != "https://github.com/owner/repo" {
			t.Errorf("GitURL = %q, want %q", dst.Spec.Source.GitURL, "https://github.com/owner/repo")
		}
		if len(dst.Spec.Source.Versions) != 0 {
			t.Errorf("Versions len = %d, want 0", len(dst.Spec.Source.Versions))
		}
		if dst.Annotations[mmconst.MintMakerOldCRDAnnotation] != "true" {
			t.Errorf("old-crd = %q, want true", dst.Annotations[mmconst.MintMakerOldCRDAnnotation])
		}
		if _, ok := dst.Annotations[mmconst.MintMakerConvertedApplicationAnnotation]; ok {
			t.Errorf("converted-application present, want absent")
		}
	})

	t.Run("old model with empty GitSource URL", func(t *testing.T) {
		src := &appstudiov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
			Spec: appstudiov1alpha1.ComponentSpec{
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitSource: &appstudiov1alpha1.GitSource{URL: ""},
					},
				},
			},
		}
		dst := ToKonfluxComponent(src)
		if dst.Spec.Source.GitURL != "" {
			t.Errorf("GitURL = %q, want empty", dst.Spec.Source.GitURL)
		}
	})

	t.Run("new model", func(t *testing.T) {
		src := &appstudiov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
			Spec: appstudiov1alpha1.ComponentSpec{
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitURL: "https://github.com/owner/repo2",
						Versions: []appstudiov1alpha1.ComponentVersion{
							{Name: "main-version", Revision: "main"},
							{Name: "", Revision: "release-1.0"}, // Name falls back to Revision
						},
					},
				},
			},
		}
		dst := ToKonfluxComponent(src)
		if dst.Spec.Source.GitURL != "https://github.com/owner/repo2" {
			t.Errorf("GitURL = %q, want %q", dst.Spec.Source.GitURL, "https://github.com/owner/repo2")
		}
		if len(dst.Spec.Source.Versions) != 2 {
			t.Fatalf("Versions len = %d, want 2", len(dst.Spec.Source.Versions))
		}
		if dst.Spec.Source.Versions[0].Name != "main-version" || dst.Spec.Source.Versions[0].Revision != "main" {
			t.Errorf("Versions[0] = %+v", dst.Spec.Source.Versions[0])
		}
		if dst.Spec.Source.Versions[1].Name != "release-1.0" || dst.Spec.Source.Versions[1].Revision != "release-1.0" {
			t.Errorf("Versions[1] = %+v", dst.Spec.Source.Versions[1])
		}
		if _, ok := dst.Annotations[mmconst.MintMakerConvertedApplicationAnnotation]; ok {
			t.Errorf("converted-application present, want absent")
		}
		if _, ok := dst.Annotations[mmconst.MintMakerOldCRDAnnotation]; ok {
			t.Errorf("old-crd present, want absent")
		}
	})
}
