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
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ghcomponent "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
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

	// Set creation timestamp for testing ordering
	if creationTimeOffset != 0 {
		pipelinerun.CreationTimestamp = metav1.NewTime(time.Now().Add(-creationTimeOffset))
	}

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

		It("should successfully launch new pipelineruns based on max limit", func() {
			// Create 3 PipelineRuns
			for i := range 3 {
				pplrName := "pplnr" + strconv.Itoa(i)
				setupPipelineRun(pplrName, nil, 0)
			}
			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(3))

			// Only MaxSimultaneousPipelineRuns should be started
			Eventually(func() int {
				count := 0
				existingPipelineRuns := listPipelineRuns(MintMakerNamespaceName)
				for _, pipelineRun := range existingPipelineRuns {
					if pipelineRun.Spec.Status == "" {
						count += 1
					}
				}
				return count
			}, timeout, interval).Should(Equal(MaxSimultaneousPipelineRuns))
		})

		It("should launch pipelineruns in order of creation time", func() {
			// Create 3 PipelineRuns with different creation times
			setupPipelineRun("oldest", nil, 30*time.Minute)
			setupPipelineRun("middle", nil, 15*time.Minute)
			setupPipelineRun("newest", nil, 5*time.Minute)

			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(3))

			// The oldest should be started first, followed by middle
			Eventually(func() bool {
				oldest := &tektonv1.PipelineRun{}
				middle := &tektonv1.PipelineRun{}
				newest := &tektonv1.PipelineRun{}

				k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "oldest"}, oldest)
				k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "middle"}, middle)
				k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "newest"}, newest)

				return oldest.Spec.Status == "" && middle.Spec.Status == "" && newest.Spec.Status == tektonv1.PipelineRunSpecStatusPending
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling GitHub token refresh", func() {
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
		})

		_ = AfterEach(func() {
			MaxSimultaneousPipelineRuns = originalMaxSimultaneousPipelineRuns
			ghcomponent.GetTokenFn = origGetTokenFn
			deleteComponent(types.NamespacedName{
				Name: labels[MintMakerComponentNameLabel], Namespace: labels[MintMakerComponentNamespaceLabel],
			})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pipelines-as-code-secret"})
			deleteSecret(types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"})

			teardownPipelineRuns()
		})

		It("should renew github token if it became old", func() {
			// Set up initial token in the secret
			appSecret := corev1.Secret{}
			Expect(
				k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, &appSecret),
			).To(Succeed())
			originalAppSecret := appSecret.DeepCopy()
			appSecret.Data = map[string][]byte{
				"renovate-token": []byte("oldtoken"),
			}
			secretPatch := client.MergeFrom(originalAppSecret)
			Expect(
				k8sClient.Patch(ctx, &appSecret, secretPatch),
			).To(Succeed())

			// Create the PipelineRun with GitHub labels
			setupPipelineRun("pplnr1", labels, 0)
			Expect(listPipelineRuns(MintMakerNamespaceName)).Should(HaveLen(1))

			// Verify the token gets updated
			Eventually(func() []byte {
				updatedSecret := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, updatedSecret)
				if err != nil {
					return []byte{}
				}
				return updatedSecret.Data["renovate-token"]
			}, timeout, interval).Should(Equal([]byte("tokenstring")))

			// Verify the PipelineRun is started
			Eventually(func() tektonv1.PipelineRunSpecStatus {
				pipelineRun := &tektonv1.PipelineRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: MintMakerNamespaceName, Name: "pplnr1"}, pipelineRun)
				if err != nil {
					return ""
				}
				return pipelineRun.Spec.Status
			}, timeout, interval).Should(Equal(tektonv1.PipelineRunSpecStatus("")))
		})
	})
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
				g.Expect(plr.IsPending()).To(BeFalse())
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
