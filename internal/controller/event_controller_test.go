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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ghcomponent "github.com/konflux-ci/mintmaker/internal/component/github"
	. "github.com/konflux-ci/mintmaker/internal/constant"
)

var _ = Describe("Event Controller", func() {

	var (
		origGetTokenFn func() (string, error)
	)

	// Test both component model versions: v1 (old model with GitSource) and v2 (new model with GitURL)
	componentModelVersions := []string{"v1", "v2"}

	for _, crdVersion := range componentModelVersions {
		crdVersion := crdVersion // capture range variable

		Context("When reconciling an event with component model "+crdVersion, func() {
			const (
				componentName      = "test-component"
				componentNamespace = "test-namespace"
				podName            = "test-pod"
				secretName         = "test-secret"
				volumeName         = "test-volume"
			)

			var (
				pod    *corev1.Pod
				secret *corev1.Secret
			)

			BeforeEach(func() {
				origGetTokenFn = ghcomponent.GetTokenFn
				ghcomponent.GetTokenFn = func() (string, error) {
					return "fake-token", nil
				}

				createNamespace(MintMakerNamespaceName)
				createNamespace(componentNamespace)

				// Create the pipelines-as-code-secret
				pacSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipelines-as-code-secret",
						Namespace: MintMakerNamespaceName,
					},
					Data: map[string][]byte{
						"github-application-id": []byte("12345"),
						"github-private-key":    []byte(testPrivateKey),
					},
				}
				Expect(k8sClient.Create(ctx, pacSecret)).Should(Succeed())

				componentKey := types.NamespacedName{Name: componentName, Namespace: componentNamespace}
				createComponent(
					componentKey, crdVersion, "app", "https://github.com/testcomp.git", "gitrevision", "gitsourcecontext",
				)

				pod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podName,
						Namespace: MintMakerNamespaceName,
						Labels: map[string]string{
							MintMakerComponentNameLabel:      componentName,
							MintMakerComponentNamespaceLabel: componentNamespace,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test"}},
						Volumes: []corev1.Volume{
							{
								Name: volumeName,
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: secretName,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: MintMakerNamespaceName,
					},
					Data: map[string][]byte{},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			})

			AfterEach(func() {
				ghcomponent.GetTokenFn = origGetTokenFn
				deleteEvents(MintMakerNamespaceName)
				Expect(k8sClient.Delete(ctx, pod)).Should(Succeed())
				Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
				deleteComponent(types.NamespacedName{Name: componentName, Namespace: componentNamespace})
				deleteSecret(types.NamespacedName{Name: "pipelines-as-code-secret", Namespace: MintMakerNamespaceName})
				deletePipelineRun(types.NamespacedName{Name: "test-pr", Namespace: MintMakerNamespaceName})
			})

			It("should successfully add the renovate token", func() {
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      podName,
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: `MountVolume.SetUp failed for volume "` + volumeName + `" : references non-existent secret key: renovate-token`,
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())

				Eventually(func() (string, error) {
					updatedSecret := &corev1.Secret{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret); err != nil {
						return "", err
					}
					return string(updatedSecret.Data["renovate-token"]), nil
				}, time.Second*10).Should(Equal("fake-token"))
			})

			It("should ignore event for non-pod object", func() {
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-non-pod",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Deployment",
						Name:      "some-deployment",
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: "some message",
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())
				Consistently(func() map[string][]byte {
					updatedSecret := &corev1.Secret{}
					_ = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret)
					return updatedSecret.Data
				}).ShouldNot(HaveKey("renovate-token"))
			})

			It("should ignore event with non-matching message", func() {
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-bad-msg",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      podName,
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: "another error message",
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())
				Consistently(func() map[string][]byte {
					updatedSecret := &corev1.Secret{}
					_ = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret)
					return updatedSecret.Data
				}).ShouldNot(HaveKey("renovate-token"))
			})

			It("should ignore event for a pod that no longer exists", func() {
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-deleted-pod",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      "deleted-pod",
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: `MountVolume.SetUp failed for volume "` + volumeName + `" : references non-existent secret key: renovate-token`,
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())
				Consistently(func() map[string][]byte {
					updatedSecret := &corev1.Secret{}
					_ = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret)
					return updatedSecret.Data
				}).ShouldNot(HaveKey("renovate-token"))
			})

			It("should ignore event for a pod where the volume does not have a corresponding secret", func() {
				podWithoutSecretVolume := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-no-secret-volume",
						Namespace: MintMakerNamespaceName,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test"}},
						Volumes: []corev1.Volume{
							{
								Name: volumeName,
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "some-configmap",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, podWithoutSecretVolume)).Should(Succeed())

				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-no-secret-volume",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      podWithoutSecretVolume.Name,
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: `MountVolume.SetUp failed for volume "` + volumeName + `" : references non-existent secret key: renovate-token`,
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())
				Consistently(func() map[string][]byte {
					updatedSecret := &corev1.Secret{}
					_ = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret)
					return updatedSecret.Data
				}).ShouldNot(HaveKey("renovate-token"))

				Expect(k8sClient.Delete(ctx, podWithoutSecretVolume)).Should(Succeed())
			})

			It("should ignore an event that has already been processed", func() {
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-processed",
						Namespace: MintMakerNamespaceName,
						Annotations: map[string]string{
							"mintmaker.appstudio.redhat.com/processed": "true",
						},
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      podName,
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: `MountVolume.SetUp failed for volume "` + volumeName + `" : references non-existent secret key: renovate-token`,
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())

				Consistently(func() map[string][]byte {
					updatedSecret := &corev1.Secret{}
					_ = k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: MintMakerNamespaceName}, updatedSecret)
					return updatedSecret.Data
				}).ShouldNot(HaveKey("renovate-token"))
			})

			It("should cancel pipelinerun when token generation fails", func() {
				const prName = "test-pr"
				pr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prName,
						Namespace: MintMakerNamespaceName,
					},
					Spec: tektonv1.PipelineRunSpec{
						// Using PipelineRef for test convenience only, it provides
						// a simple way to create a PipelineRun
						PipelineRef: &tektonv1.PipelineRef{
							Name: "test-pipeline",
						},
					},
				}
				Expect(k8sClient.Create(ctx, pr)).Should(Succeed())
				pod.Labels["tekton.dev/pipelineRun"] = prName
				Expect(k8sClient.Update(ctx, pod)).Should(Succeed())

				ghcomponent.GetTokenFn = func() (string, error) {
					return "", errors.New("token error")
				}

				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-event-for-pr",
						Namespace: MintMakerNamespaceName,
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      "Pod",
						Name:      podName,
						Namespace: MintMakerNamespaceName,
					},
					Reason:  "FailedMount",
					Message: `MountVolume.SetUp failed for volume "` + volumeName + `" : references non-existent secret key: renovate-token`,
				}
				Expect(k8sClient.Create(ctx, event)).Should(Succeed())

				Eventually(func() string {
					updatedPR := &tektonv1.PipelineRun{}
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: prName, Namespace: MintMakerNamespaceName}, updatedPR); err != nil {
						return ""
					}
					return string(updatedPR.Spec.Status)
				}, time.Second*10).Should(Equal(string(tektonv1.PipelineRunSpecStatusCancelled)))
			})
		})
	}
})

func deleteEvents(namespace string) {
	eventList := &corev1.EventList{}
	err := k8sClient.List(ctx, eventList, client.InNamespace(namespace))
	Expect(err).NotTo(HaveOccurred())
	for _, event := range eventList.Items {
		err := k8sClient.Delete(ctx, &event)
		Expect(err).NotTo(HaveOccurred())
	}
}
