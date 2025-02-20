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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	ghcomponent "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
)

var _ = Describe("DependencyUpdateCheck Controller", func() {

	var (
		origGetRenovateConfig func(registrySecret *corev1.Secret) (string, error)
		origGetTokenFn        func() (string, error)
	)

	Context("Test Renovate jobs creation", func() {

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
			createNamespace("testnamespace")
			createComponent(
				types.NamespacedName{Name: "testcomp", Namespace: "testnamespace"}, "app", "https://github.com/testcomp.git", "gitrevision", "gitsourcecontext",
			)
			secretData := map[string]string{
				"github-application-id": "1234567890",
				"github-private-key":    testPrivateKey,
			}
			createSecret(
				types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"}, secretData,
			)
			configMapData := map[string]string{"renovate.json": "{}"}
			createConfigMap(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "renovate-config"}, configMapData)

			origGetRenovateConfig = ghcomponent.GetRenovateConfigFn
			ghcomponent.GetRenovateConfigFn = func(registrySecret *corev1.Secret) (string, error) {
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
			deleteComponent(types.NamespacedName{Name: "testcomp", Namespace: "testnamespace"})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"})
			deleteConfigMap(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "renovate-config"})
			ghcomponent.GetRenovateConfigFn = origGetRenovateConfig
			ghcomponent.GetTokenFn = origGetTokenFn
		})

		It("should create a pipeline run when a CR DependencyUpdateCheck is created", func() {
			dependencyUpdateCheckKey := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "dependencyupdatecheck-sample"}
			createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(1))
			deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
		})

		It("should not create a pipelinerun for DependencyUpdateCheck CR which has been processed before", func() {
			// Create a DependencyUpdateCheck CR in "mintmaker" namespace, that was processed before
			dependencyUpdateCheckKey := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "dependencyupdatecheck-sample"}
			createDependencyUpdateCheck(dependencyUpdateCheckKey, true, nil)
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(0))
			deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
		})

		It("should not create a pipelinerun for DependencyUpdateCheck CR that is not from mintmaker namespace", func() {
			// Create a DependencyUpdateCheck CR in "mintmaker" namespace, that was processed before
			dependencyUpdateCheckKey := types.NamespacedName{Namespace: "default", Name: "dependencyupdatecheck-sample"}
			createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(0))
			deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
		})
	})
})
