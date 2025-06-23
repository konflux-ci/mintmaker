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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ghcomponent "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
	"github.com/konflux-ci/mintmaker/internal/pkg/config"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
	tekton "github.com/konflux-ci/mintmaker/internal/pkg/tekton"
)

func setupPipelineRun(name string, labels map[string]string) {
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

var _ = Describe("PipelineRun Controller", FlakeAttempts(5), func() {

	MaxSimultaneousPipelineRuns := config.GetTestConfig().PipelineRunConfig.MaxParallelPipelineruns

	Context("When reconciling pipelineruns", func() {

		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns

		_ = BeforeEach(func() {
			createNamespace(MintMakerNamespaceName)
			MaxSimultaneousPipelineRuns = 2
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns

			teardownPipelineRuns()
		})

		It("should successfully launch new pipelineruns", func() {

			for i := range 3 {
				pplrName := "pplnr" + strconv.Itoa(i)
				setupPipelineRun(pplrName, nil)
			}
			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(3))

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
		var origGetTokenFn func() (string, error)
		originalMaxSimultaneousPipelineRuns := MaxSimultaneousPipelineRuns
		labels := map[string]string{
			MintMakerGitPlatformLabel:        "github",
			MintMakerComponentNameLabel:      "testcomp",
			MintMakerComponentNamespaceLabel: "testnamespace",
		}

		_ = BeforeEach(func() {
			MaxSimultaneousPipelineRuns = 1
			createNamespace(MintMakerNamespaceName)
			createNamespace("testnamespace")
			origGetTokenFn = ghcomponent.GetTokenFn
			ghcomponent.GetTokenFn = func() (string, error) {
				return "tokenstring", nil
			}

			createComponent(
				types.NamespacedName{
					Name: labels[MintMakerComponentNameLabel], Namespace: labels[MintMakerComponentNamespaceLabel],
				}, "app", "https://github.com/testcomp.git", "gitrevision", "gitsourcecontext",
			)

			secretData := map[string]string{
				"github-application-id": "1234567890",
				"github-private-key":    testPrivateKey,
			}
			createSecret(
				types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"}, secretData,
			)
			configMapData := map[string]string{"renovate.json": "{}"}
			createSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, configMapData)
			createSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr2"}, configMapData)

		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns
			ghcomponent.GetTokenFn = origGetTokenFn
			deleteComponent(types.NamespacedName{
				Name: labels[MintMakerComponentNameLabel], Namespace: labels[MintMakerComponentNamespaceLabel],
			})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr2"})

			pplr := tektonv1.PipelineRun{}
			k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, &pplr)
			teardownPipelineRuns()
		})

		It("should launch 'github' pipelines first", func() {

			var flag1, flag2 bool = false, false

			setupPipelineRun("pplnr1", labels)
			setupPipelineRun("pplnr2", nil)
			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(2))

			pipelineRuns := listPipelineRuns(MintMakerNamespaceName)
			for _, plr := range pipelineRuns {
				if plr.Labels[MintMakerGitPlatformLabel] == "github" {
					Eventually(plr.Spec.Status).Should(BeEmpty())
					flag1 = true
				} else {
					Consistently(string(plr.Spec.Status)).Should(Equal(tektonv1.PipelineRunSpecStatusPending))
					flag2 = true
				}
			}
			Expect(flag1 && flag2).To(BeTrue()) //make sure both clauses were executed
		})

		It("should renew github token if it became old", func() {
			appSecret := corev1.Secret{}
			Expect(
				k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, &appSecret),
			).To(Succeed())
			originalAppSecret := appSecret.DeepCopy()
			appSecret.Data["renovate-token"] = []byte("oldtoken")
			secretPatch := client.MergeFrom(originalAppSecret)
			Expect(
				k8sClient.Patch(ctx, &appSecret, secretPatch),
			).To(Succeed())

			setupPipelineRun("pplnr1", labels)
			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(1))

			// check that secret is updated with new token; for some weird reason, getSecret did not return the updated value
			Eventually(
				func() []byte {
					finalSecret := corev1.Secret{}
					k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, &finalSecret)
					return finalSecret.Data["renovate-token"]
				}(),
			).Should(Equal([]byte("tokenstring")))

		})
	})
})
