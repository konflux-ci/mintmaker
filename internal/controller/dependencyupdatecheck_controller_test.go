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
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/component"
	"github.com/konflux-ci/mintmaker/internal/component/mocks"
	. "github.com/konflux-ci/mintmaker/internal/constant"
)

var _ = Describe("DependencyUpdateCheck Controller", func() {

	// Test both component model versions: v1 (old model with GitSource) and v2 (new model with GitURL)
	componentModelVersions := []string{"v1", "v2"}

	for _, crdVersion := range componentModelVersions {
		crdVersion := crdVersion // capture range variable

		// v1 model has a single revision, v2 model has 3 versions defined in createComponent
		numExpectedPipelineRuns := 1
		if crdVersion == "v2" {
			numExpectedPipelineRuns = 3
		}
		expectedPipelineRuns := numExpectedPipelineRuns // capture for closures

		Context("When reconciling a DependencyUpdateCheck CR with component model "+crdVersion, func() {

			const (
				dependencyUpdateCheckName = "dependencyupdatecheck-sample"
				componentName             = "testcomp"
				componentNamespace        = "testnamespace"
			)

			dependencyUpdateCheckKey := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: dependencyUpdateCheckName}

			_ = BeforeEach(func() {
				createNamespace(MintMakerNamespaceName)
				createNamespace(componentNamespace)
				createComponent(
					types.NamespacedName{Name: componentName, Namespace: componentNamespace}, crdVersion, "app", "https://github.com/testcomp.git", "gitrevision", "gitsourcecontext",
				)

				gt := GinkgoT()
				newGitComponentForTest = func(_ context.Context, appComp *appstudiov1alpha1.Component, _ client.Client) (component.GitComponent, error) {
					mockComp := mocks.NewMockGitComponent(gt)
					branches := component.GetVersions(appComp)
					if len(branches) == 0 {
						branches = []string{"main"}
					}
					mockComp.EXPECT().GetBranches().Return(branches).Maybe()
					mockComp.EXPECT().GetName().Return(appComp.Name).Maybe()
					mockComp.EXPECT().GetNamespace().Return(appComp.Namespace).Maybe()
					mockComp.EXPECT().GetApplication().Return(appComp.Spec.Application).Maybe()
					mockComp.EXPECT().GetPlatform().Return("github").Maybe()
					mockComp.EXPECT().GetHost().Return("github.com").Maybe()
					mockComp.EXPECT().GetRepository().Return("testcomp").Maybe()
					mockComp.EXPECT().GetRenovateConfig(mock.Anything, mock.Anything).Return("mock config", nil).Maybe()
					mockComp.EXPECT().GetRPMActivationKey(mock.Anything, mock.Anything).Return("", "", fmt.Errorf("no rpm key")).Maybe()
					return mockComp, nil
				}

				Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(0))
			})

			_ = AfterEach(func() {
				deletePipelineRuns(MintMakerNamespaceName)
				deleteComponent(types.NamespacedName{Name: componentName, Namespace: componentNamespace})
			})

			It("should create pipelineruns for each branch/version", func() {
				createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)
				Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(expectedPipelineRuns))
				deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
			})

			It("should not create a pipelinerun if the DependencyUpdateCheck CR has been processed before", func() {
				// Create a DependencyUpdateCheck CR in "mintmaker" namespace, that was processed before
				createDependencyUpdateCheck(dependencyUpdateCheckKey, true, nil)
				Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(0))
				deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
			})

			It("should not create a pipelinerun if the DependencyUpdateCheck CR is not from the mintmaker namespace", func() {
				createDependencyUpdateCheck(types.NamespacedName{Namespace: "default", Name: dependencyUpdateCheckName}, false, nil)
				Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(0))
				deleteDependencyUpdateCheck(types.NamespacedName{Namespace: "default", Name: dependencyUpdateCheckName})
			})

			Context("When getting a merged docker config for a pipelinerun", func() {

				const (
					serviceAccountName = "build-pipeline-" + componentName
					registrySecretName = "testregistrysecret"
				)

				var mergedConfigJson []byte
				registrySecretKey := types.NamespacedName{Namespace: componentNamespace, Name: registrySecretName}
				serviceAccountKey := types.NamespacedName{Namespace: componentNamespace, Name: serviceAccountName}

				_ = BeforeEach(func() {
					createServiceAccount(serviceAccountKey)

					// Create a merged docker config
					mergedAuths := map[string]interface{}{
						"https://fake-registry.com": map[string]string{"auth": "fake-auth"},
					}
					var err error
					mergedConfigJson, err = json.Marshal(map[string]interface{}{"auths": mergedAuths})
					Expect(err).NotTo(HaveOccurred())
				})

				_ = AfterEach(func() {
					deleteServiceAccount(serviceAccountKey)
				})

				It("should include secrets linked to the build-pipeline service account", func() {
					// Create image registry secret
					secretData := map[string][]byte{corev1.DockerConfigJsonKey: mergedConfigJson}
					createSecret(registrySecretKey, corev1.SecretTypeDockerConfigJson, secretData, nil)

					// Link the registry secret to the service account
					serviceAccount := getServiceAccount(serviceAccountKey)
					serviceAccount.Secrets = []corev1.ObjectReference{{Name: registrySecretName}}
					Expect(k8sClient.Update(ctx, serviceAccount)).Should(Succeed())

					createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)

					Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(expectedPipelineRuns))
					Eventually(func() map[string][]byte {
						plrName := listPipelineRuns(MintMakerNamespaceName)[0].Name
						renovateSecret := getSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plrName})
						return renovateSecret.Data
					}).Should(HaveKeyWithValue(corev1.DockerConfigJsonKey, mergedConfigJson))

					deleteSecret(registrySecretKey)
					deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
				})

				It("should exclude secrets that are not linked to the build-pipeline service account", func() {
					// Create image registry secret
					secretData := map[string][]byte{corev1.DockerConfigJsonKey: mergedConfigJson}
					createSecret(registrySecretKey, corev1.SecretTypeDockerConfigJson, secretData, nil)

					createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)

					Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(expectedPipelineRuns))
					Consistently(func() map[string][]byte {
						plrName := listPipelineRuns(MintMakerNamespaceName)[0].Name
						renovateSecret := getSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plrName})
						return renovateSecret.Data
					}).Should(BeNil())

					deleteSecret(registrySecretKey)
					deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
				})

				It("should exclude secrets that are not of the DockerConfigJson type", func() {
					// Create image registry secret
					secretData := map[string][]byte{corev1.BasicAuthUsernameKey: []byte("testusername"), corev1.BasicAuthPasswordKey: []byte("testpassword")}
					createSecret(registrySecretKey, corev1.SecretTypeBasicAuth, secretData, nil)

					// Link the registry secret to the service account
					serviceAccount := getServiceAccount(serviceAccountKey)
					serviceAccount.Secrets = []corev1.ObjectReference{{Name: registrySecretName}}
					Expect(k8sClient.Update(ctx, serviceAccount)).Should(Succeed())

					createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)

					Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(expectedPipelineRuns))
					Consistently(func() map[string][]byte {
						plrName := listPipelineRuns(MintMakerNamespaceName)[0].Name
						renovateSecret := getSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plrName})
						return renovateSecret.Data
					}).Should(BeNil())

					deleteSecret(registrySecretKey)
					deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
				})

				It("should exclude secrets that are labeled as internal", func() {
					// Create image registry secret
					secretData := map[string][]byte{corev1.DockerConfigJsonKey: mergedConfigJson}
					createSecret(registrySecretKey, corev1.SecretTypeDockerConfigJson, secretData, nil)

					// Add the internal label to the registry secret
					registrySecret := getSecret(registrySecretKey)
					registrySecret.Labels = map[string]string{InternalSecretLabelName: "true"}
					Expect(k8sClient.Update(ctx, registrySecret)).Should(Succeed())

					// Link the registry secret to the service account
					serviceAccount := getServiceAccount(serviceAccountKey)
					serviceAccount.Secrets = []corev1.ObjectReference{{Name: registrySecretName}}
					Expect(k8sClient.Update(ctx, serviceAccount)).Should(Succeed())

					createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)

					Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(expectedPipelineRuns))
					Consistently(func() map[string][]byte {
						plrName := listPipelineRuns(MintMakerNamespaceName)[0].Name
						renovateSecret := getSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plrName})
						return renovateSecret.Data
					}).Should(BeNil())

					deleteSecret(registrySecretKey)
					deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
				})
			})
		})
	}
})
