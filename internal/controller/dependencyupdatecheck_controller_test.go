/*
Copyright 2024 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/konflux-ci/mintmaker/pkg/common"
)

/*
	WHAT TO TEST:

- should not create a pipelinerun for DependencyUpdateCheck CR which has been processed before
- should create a pipeline run when a CR DependencyUpdateCheck is created
- should not create a pipelinerun for DependencyUpdateCheck CR that is not from mintmaker namespace
*/
var _ = Describe("DependencyUpdateCheck Controller", func() {

	var ()

	Context("Test Renovate jobs creation", func() {

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
		})

		_ = AfterEach(func() {
		})

		It("should create a pipeline run when a CR DependencyUpdateCheck is created", func() {
			// Create a DependencyUpdateCheck CR in "mintmaker" namespace
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
			dependencyUpdateCheckKey := types.NamespacedName{Namespace: "wrong-namespace", Name: "dependencyupdatecheck-sample"}
			createDependencyUpdateCheck(dependencyUpdateCheckKey, false, nil)
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(0))
			deleteDependencyUpdateCheck(dependencyUpdateCheckKey)
		})
	})
})
