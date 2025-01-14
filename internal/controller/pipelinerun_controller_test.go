/*
Copyright 2024.

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

// What to test:
// - check countRunningPipelineRuns
// - with a queue of 3 pipelineruns, when one finishes one other starts and one stays pending

package controller

import (
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	local_tekton "github.com/konflux-ci/mintmaker/internal/pkg/tekton"
	. "github.com/konflux-ci/mintmaker/pkg/common"
)

var _ = Describe("PipelineRun Controller", FlakeAttempts(3), func() {
	Context("When reconciling pipelineruns", func() {

		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
			MaxSimultaneousPipelineRuns = 2

			for i := range 3 {
				pplrName := "pplnr" + strconv.Itoa(i)
				pipelineRunBuilder := local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
				pipelinerun, err := pipelineRunBuilder.Build()
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Create(ctx, pipelinerun)).Should(Succeed())
			}
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(3))
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns

			// Delete pipelineruns, so they don't leak to other tests
			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, pipelinerun := range pipelineRuns {
				Expect(k8sClient.Delete(ctx, &pipelinerun)).Should(Succeed())
			}
		})

		It("should successfully launch new pipelineruns", func() {

			existingPipelineRuns := tektonv1.PipelineRunList{
				Items: listPipelineRuns(MintMakerNamespaceName),
			}
			count := 0
			for _, pipelineRun := range existingPipelineRuns.Items {
				if pipelineRun.Spec.Status == "" {
					count += 1
				}
			}
			Expect(count).Should(Equal(MaxSimultaneousPipelineRuns))
		})
	})

	Context("When launching new pipelineruns", func() {

		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
			MaxSimultaneousPipelineRuns = 1

			pplrName := "pplnr1"
			labels := make(map[string]string)
			labels[MintMakerGitPlatformLabel] = "github"
			pipelineRunBuilder := local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
			pipelinerun, err := pipelineRunBuilder.WithLabels(labels).Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, pipelinerun)).Should(Succeed())

			pplrName = "pplnr2"
			pipelineRunBuilder = local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
			pipelinerun, err = pipelineRunBuilder.Build()
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, pipelinerun)).Should(Succeed())

			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(2))
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns

			// Delete pipelineruns, so they don't leak to other tests
			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, pipelinerun := range pipelineRuns {
				Expect(k8sClient.Delete(ctx, &pipelinerun)).Should(Succeed())
			}
		})
		// FIXME improve this test
		// see discussion in https://github.com/konflux-ci/mintmaker/pull/132
		It("should launch 'github' pipelines first", func() {

			var flag1, flag2 bool = false, false

			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, plr := range pipelineRuns {
				if plr.Labels[MintMakerGitPlatformLabel] == "github" {
					Expect(plr.Spec.Status).To(BeEmpty())
					flag1 = true
				} else {
					Expect(string(plr.Spec.Status)).To(Equal(tektonv1.PipelineRunSpecStatusPending))
					flag2 = true
				}
			}
			Expect(flag1 && flag2).To(BeTrue()) //make sure both clauses were executed
		})
	})
})
