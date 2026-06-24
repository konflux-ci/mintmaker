// Copyright 2024 Red Hat, Inc.
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

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativeapis "knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"

	. "github.com/konflux-ci/mintmaker/internal/constant"
)

func createNamespace(name string) {
	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := k8sClient.Create(ctx, namespace); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}

	Eventually(func() bool {
		namespace := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, namespace)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

func createServiceAccount(resourceKey types.NamespacedName) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
	}
	if err := k8sClient.Create(ctx, sa); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteServiceAccount(resourceKey)
		Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
	}
	getServiceAccount(resourceKey)
}

func deleteServiceAccount(resourceKey types.NamespacedName) {
	sa := &corev1.ServiceAccount{}
	if err := k8sClient.Get(ctx, resourceKey, sa); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, sa); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, sa))
	}, timeout, interval).Should(BeTrue())
}

func getServiceAccount(resourceKey types.NamespacedName) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{}
	Eventually(func() error {
		return k8sClient.Get(ctx, resourceKey, sa)
	}, timeout, interval).Should(Succeed())
	return sa
}

func createSecret(resourceKey types.NamespacedName, secretType corev1.SecretType, data map[string][]byte, stringData map[string]string) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		Type:       secretType,
		Data:       data,
		StringData: stringData,
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteSecret(resourceKey)
		secret.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	}

	getSecret(resourceKey)
}

func deleteSecret(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, resourceKey, secret); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, secret); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, secret))
	}, timeout, interval).Should(BeTrue())
}

func getSecret(resourceKey types.NamespacedName) *corev1.Secret {
	secret := &corev1.Secret{}
	Eventually(func() error {
		return k8sClient.Get(ctx, resourceKey, secret)
	}, timeout, interval).Should(Succeed())
	return secret
}

func createComponent(resourceKey types.NamespacedName, crdVersion, application, gitURL, gitRevision, gitSourceContext string) {
	if crdVersion == "v1" {
		component := &appstudiov1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceKey.Name,
				Namespace: resourceKey.Namespace,
				Annotations: map[string]string{
					"appstudio.openshift.io/component-model": crdVersion,
				},
			},
			Spec: appstudiov1alpha1.ComponentSpec{
				ComponentName: resourceKey.Name,
				Application:   application,
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitSource: &appstudiov1alpha1.GitSource{
							URL:      gitURL,
							Revision: gitRevision,
							Context:  gitSourceContext,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, component)).Should(Succeed())
		getComponent(resourceKey)
	} else {
		component := &appstudiov1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceKey.Name,
				Namespace: resourceKey.Namespace,
				Annotations: map[string]string{
					"appstudio.openshift.io/component-model": crdVersion,
				},
			},
			Spec: appstudiov1alpha1.ComponentSpec{
				Source: appstudiov1alpha1.ComponentSource{
					ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
						GitURL: gitURL,
						Versions: []appstudiov1alpha1.ComponentVersion{
							{
								Name:     "default",
								Revision: gitRevision,
							},
							{
								Name:     "v1",
								Revision: gitRevision + "-v1",
							},
							{
								Name:     "v2",
								Revision: gitRevision + "-v2",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, component)).Should(Succeed())

		createdComponent := getComponent(resourceKey)
		createdComponent.Status = appstudiov1alpha1.ComponentStatus{
			Versions: []appstudiov1alpha1.ComponentVersionStatus{
				{Name: "default", Revision: gitRevision, OnboardingStatus: "succeeded"},
				{Name: "v1", Revision: gitRevision + "-v1", OnboardingStatus: "succeeded"},
				{Name: "v2", Revision: gitRevision + "-v2", OnboardingStatus: "succeeded"},
			},
		}
		Expect(k8sClient.Status().Update(ctx, createdComponent)).Should(Succeed())
		getComponent(resourceKey)
	}
}

func getComponent(resourceKey types.NamespacedName) *appstudiov1alpha1.Component {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, resourceKey, component); err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())
	return component
}

// deleteComponent deletes the specified component resource and verifies it was properly deleted
func deleteComponent(resourceKey types.NamespacedName) {
	component := &appstudiov1alpha1.Component{}

	// Check if the component exists
	if err := k8sClient.Get(ctx, resourceKey, component); k8sErrors.IsNotFound(err) {
		return
	}

	// Delete
	Eventually(func() error {
		if err := k8sClient.Get(ctx, resourceKey, component); err != nil {
			return err
		}
		return k8sClient.Delete(ctx, component)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, component))
	}, timeout, interval).Should(BeTrue())
}

func createDependencyUpdateCheck(resourceKey types.NamespacedName, processed bool, namespaces []mmv1alpha1.NamespaceSpec) {
	annotations := map[string]string{}
	if processed {
		annotations[MintMakerProcessedAnnotationName] = "true"
	}

	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "DependencyUpdateCheck",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceKey.Name,
			Namespace:   resourceKey.Namespace,
			Annotations: annotations,
		},
		Spec: mmv1alpha1.DependencyUpdateCheckSpec{},
	}
	if len(namespaces) > 0 {
		dependencyUpdateCheck.Spec.Namespaces = namespaces
	}

	Expect(k8sClient.Create(ctx, dependencyUpdateCheck)).Should(Succeed())
	getDependencyUpdateCheck(resourceKey)
}

func getDependencyUpdateCheck(resourceKey types.NamespacedName) *mmv1alpha1.DependencyUpdateCheck {
	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck); err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())
	return dependencyUpdateCheck
}

func deleteDependencyUpdateCheck(resourceKey types.NamespacedName) {
	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{}
	if err := k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, dependencyUpdateCheck); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck))
	}, timeout, interval).Should(BeTrue())
}

func createMintmakerPipelineRun(name, namespace string, labels map[string]string, succeeded corev1.ConditionStatus) {
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineSpec: &tektonv1.PipelineSpec{
				Tasks: []tektonv1.PipelineTask{
					{
						Name: "noop",
						TaskSpec: &tektonv1.EmbeddedTask{
							TaskSpec: tektonv1.TaskSpec{
								Steps: []tektonv1.Step{
									{Name: "noop", Image: "busybox", Script: "echo noop"},
								},
							},
						},
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, pr)).Should(Succeed())

	// Update status with the given condition if not empty string
	if succeeded != "" {
		pr.Status.SetCondition(&knativeapis.Condition{
			Type:   knativeapis.ConditionSucceeded,
			Status: succeeded,
		})
		Expect(k8sClient.Status().Update(ctx, pr)).Should(Succeed())
	}
}

func listPipelineRuns(namespace string) []tektonv1.PipelineRun {
	pipelineruns := &tektonv1.PipelineRunList{}

	err := k8sClient.List(ctx, pipelineruns, client.InNamespace(namespace))
	Expect(err).ToNot(HaveOccurred())
	return pipelineruns.Items
}

func deletePipelineRuns(namespace string) {
	err := k8sClient.DeleteAllOf(ctx, &tektonv1.PipelineRun{}, client.InNamespace(namespace), client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		return len(listPipelineRuns(namespace)) == 0
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

func deletePipelineRun(resourceKey types.NamespacedName) {
	pipelineRun := &tektonv1.PipelineRun{}
	if err := k8sClient.Get(ctx, resourceKey, pipelineRun); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, pipelineRun); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, pipelineRun))
	}, timeout, interval).Should(BeTrue())
}
