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
	"bytes"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
	tekton "github.com/konflux-ci/mintmaker/internal/pkg/tekton"
)

func setupPipelineRun(name string, labels map[string]string, creationTimeOffset time.Duration) {
	pipelineRunBuilder := tekton.NewPipelineRunBuilder(name, MintMakerNamespaceName)
	var err error
	var pipelinerun *tektonv1.PipelineRun
	if labels != nil {
		pipelinerun, err = pipelineRunBuilder.WithLabels(labels).Build()
	} else {
		pipelinerun, err = pipelineRunBuilder.Build()
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Create(ctx, pipelinerun)).Should(Succeed())
}

func teardownPipelineRuns() {
	pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
	for _, pipelinerun := range pipelineRuns {
		Expect(k8sClient.Delete(ctx, &pipelinerun)).Should(Succeed())
	}
	Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(0))
}

var _ = Describe("PipelineRun Controller", func() {

	Context("When a pipelinerun finishes", func() {

		var logBuffer bytes.Buffer

		plrName := "test-plr"
		plrLookupKey := types.NamespacedName{Name: plrName, Namespace: MintMakerNamespaceName}
		plr := &tektonv1.PipelineRun{}

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
			setupPipelineRun(plrName, nil, 0)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, plrLookupKey, plr)).To(Succeed())
			}, timeout, interval).Should(Succeed())
			GinkgoWriter.TeeTo(&logBuffer)
		})

		_ = AfterEach(func() {
			GinkgoWriter.ClearTeeWriters()
			logBuffer.Reset()
			teardownPipelineRuns()
		})

		It("should log completion timestamp if successful", func() {

			plr.Status.MarkSucceeded(string(tektonv1.PipelineRunReasonSuccessful), "%s")
			Expect(k8sClient.Status().Update(ctx, plr)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, plrLookupKey, plr)).To(Succeed())
				g.Expect(plr.Status.CompletionTime).ToNot(Equal(nil))
			}, timeout, interval).Should(Succeed())

			expected := "PipelineRun finished: %s	{\"completionTime\": \"%s\", \"success\": true, \"reason\": \"Succeeded\"}"
			Expect(logBuffer.String()).To(ContainSubstring(expected, plr.Name, plr.Status.CompletionTime.Format(time.RFC3339)))
		})

		It("should log completion timestamp if failed", func() {

			plr.Status.MarkFailed(string(tektonv1.PipelineRunReasonFailed), "%s")
			Expect(k8sClient.Status().Update(ctx, plr)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, plrLookupKey, plr)).To(Succeed())
				g.Expect(plr.Status.CompletionTime).ToNot(Equal(nil))
			}, timeout, interval).Should(Succeed())

			expected := "PipelineRun finished: %s	{\"completionTime\": \"%s\", \"success\": false, \"reason\": \"Failed\"}"
			Expect(logBuffer.String()).To(ContainSubstring(expected, plr.Name, plr.Status.CompletionTime.Format(time.RFC3339)))
		})

		It("should log completion timestamp if cancelled", func() {

			plr.Status.MarkFailed(string(tektonv1.PipelineRunReasonCancelled), "%s")
			Expect(k8sClient.Status().Update(ctx, plr)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, plrLookupKey, plr)).To(Succeed())
				g.Expect(plr.Status.CompletionTime).ToNot(Equal(nil))
			}, timeout, interval).Should(Succeed())

			expected := "PipelineRun finished: %s	{\"completionTime\": \"%s\", \"success\": false, \"reason\": \"Cancelled\"}"
			Expect(logBuffer.String()).To(ContainSubstring(expected, plr.Name, plr.Status.CompletionTime.Format(time.RFC3339)))
		})
	})
})
