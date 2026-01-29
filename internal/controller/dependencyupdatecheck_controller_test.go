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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	ghcomponent "github.com/konflux-ci/mintmaker/internal/component/github"
	. "github.com/konflux-ci/mintmaker/internal/constant"
)

var _ = Describe("DependencyUpdateCheck Controller", func() {

	var (
		origGetRenovateConfig func(registrySecret *corev1.Secret, currentBranch string) (string, error)
		origGetTokenFn        func() (string, error)
	)

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
				secretData := map[string]string{
					"github-application-id": "1234567890",
					"github-private-key":    testPrivateKey,
				}
				createSecret(
					types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"}, corev1.SecretTypeOpaque, nil, secretData,
				)
				configMapData := map[string]string{"renovate.json": "{}"}
				createConfigMap(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "renovate-config"}, configMapData)

				origGetRenovateConfig = ghcomponent.GetRenovateConfigFn
				ghcomponent.GetRenovateConfigFn = func(registrySecret *corev1.Secret, currentBranch string) (string, error) {
					return "mock config", nil
				}

				origGetTokenFn = ghcomponent.GetTokenFn
				ghcomponent.GetTokenFn = func() (string, error) {
					return "tokenstring", nil
				}

				Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(0))
			})

			_ = AfterEach(func() {
				deletePipelineRuns(MintMakerNamespaceName)
				deleteComponent(types.NamespacedName{Name: componentName, Namespace: componentNamespace})
				deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"})
				deleteConfigMap(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "renovate-config"})
				ghcomponent.GetRenovateConfigFn = origGetRenovateConfig
				ghcomponent.GetTokenFn = origGetTokenFn
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
