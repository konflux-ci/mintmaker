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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	local_tekton "github.com/konflux-ci/mintmaker/internal/pkg/tekton"
	. "github.com/konflux-ci/mintmaker/pkg/common"
)

var _ = Describe("PipelineRun Controller", func() {
	Context("When reconciling pipelineruns", func() {

		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns

		_ = BeforeEach(func() {

			MaxSimultaneousPipelineRuns = 2

			for i := range 3 {
				pplrName := "pplnr" + strconv.Itoa(i)
				pipelineRunBuilder := local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
				pipelinerun, err := pipelineRunBuilder.Build()
				Expect(err).NotTo(HaveOccurred())
				k8sClient.Create(ctx, pipelinerun)
			}
			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(3))
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns

			// Delete pipelineruns, so they don't leak to other tests
			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, pipelinerun := range pipelineRuns {
				k8sClient.Delete(ctx, &pipelinerun)
			}
		})

		It("should successfully launch new pipelineruns", func() {

			reconciler := PipelineRunReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}
			reconciler.SetupWithManager(k8sManager)
			reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"},
			})

			existingPipelineRuns := tektonv1.PipelineRunList{
				Items: listPipelineRuns(MintMakerNamespaceName),
			}
			// To count running pipelineruns, we have to check for empty 'status' fields;
			// During the tests, these pipelineruns will not be updated to be actually 'running'
			count := 0
			for _, pipelineRun := range existingPipelineRuns.Items {
				if pipelineRun.Spec.Status == "" {
					count += 1
				}
			}
			Expect(count).To(Equal(MaxSimultaneousPipelineRuns))
		})
	})

	Context("When launching new pipelineruns", func() {

		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns

		_ = BeforeEach(func() {
			MaxSimultaneousPipelineRuns = 1

			pplrName := "pplnr1"
			labels := make(map[string]string)
			labels[MintMakerAppstudioLabel] = "github"
			pipelineRunBuilder := local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
			pipelinerun, err := pipelineRunBuilder.WithLabels(labels).Build()
			Expect(err).NotTo(HaveOccurred())
			k8sClient.Create(ctx, pipelinerun)

			pplrName = "pplnr2"
			pipelineRunBuilder = local_tekton.NewPipelineRunBuilder(pplrName, MintMakerNamespaceName)
			pipelinerun, err = pipelineRunBuilder.Build()
			Expect(err).NotTo(HaveOccurred())
			k8sClient.Create(ctx, pipelinerun)

			Eventually(listPipelineRuns).WithArguments(MintMakerNamespaceName).Should(HaveLen(2))
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns

			// Delete pipelineruns, so they don't leak to other tests
			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, pipelinerun := range pipelineRuns {
				k8sClient.Delete(ctx, &pipelinerun)
			}
		})

		It("should launch 'github' pipelines first", func() {
			reconciler := PipelineRunReconciler{
				Client: k8sClient,
				Scheme: scheme.Scheme,
			}
			reconciler.SetupWithManager(k8sManager)
			reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"},
			})

			existingPipelineRuns := tektonv1.PipelineRunList{
				Items: listPipelineRuns(MintMakerNamespaceName),
			}
			var checkPipelineRun tektonv1.PipelineRun
			for _, p := range existingPipelineRuns.Items {
				if p.ObjectMeta.Name == "pplnr1" {
					checkPipelineRun = p
					break
				}
			}
			// To count running pipelineruns, we have to check for empty 'status' fields;
			// During the tests, these pipelineruns will not be updated to be actually 'running'
			Expect(string(checkPipelineRun.Spec.Status)).To(Equal(""))
		})
	})
})
