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
	"fmt"
	"math/rand"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gh "github.com/google/go-github/v45/github"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"

	. "github.com/konflux-ci/mintmaker/pkg/common"
	"github.com/konflux-ci/mintmaker/pkg/git/github"
)

const (
	// timeout is used as a limit until condition become true
	// Usually used in Eventually statements
	timeout  = time.Second * 15
	interval = time.Millisecond * 250
)

func createNamespace(name string) {
	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := k8sClient.Create(ctx, namespace); err != nil && !k8sErrors.IsAlreadyExists(err) {
		Fail(err.Error())
	}

	Eventually(func() bool {
		namespace := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, namespace)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

func createSecret(resourceKey types.NamespacedName, data map[string]string) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		StringData: data,
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteSecret(resourceKey)
		secret.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	}

	getSecret(resourceKey)
}

func createSCMSecret(resourceKey types.NamespacedName, data map[string]string, secretType corev1.SecretType, annotations map[string]string) {
	labels := map[string]string{
		"appstudio.redhat.com/credentials": "scm",
		"appstudio.redhat.com/scm.host":    "gitlab.com",
	}
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		Type: secretType,
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceKey.Name,
			Namespace:   resourceKey.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		StringData: data,
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteSecret(resourceKey)
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	}

	getSecret(resourceKey)
}

func deleteSecret(resourceKey types.NamespacedName) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, resourceKey, secret); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, secret); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, secret))
	}, timeout, interval).Should(BeTrue())
}

func getSecret(resourceKey types.NamespacedName) {
	Eventually(func() error {
		secret := &corev1.Secret{}
		return k8sClient.Get(ctx, resourceKey, secret)
	}, timeout, interval).Should(Succeed())
}

func createConfigMap(resourceKey types.NamespacedName, data map[string]string) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		Data: data,
	}
	Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())
	getConfigMap(resourceKey)
}

func getConfigMap(resourceKey types.NamespacedName) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, resourceKey, configMap); err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())
	return configMap
}

func deleteConfigMap(resourceKey types.NamespacedName) {
	configMap := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, resourceKey, configMap); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, configMap); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, configMap))
	}, timeout, interval).Should(BeTrue())
}

func createComponent(resourceKey types.NamespacedName, application, gitURL, gitRevision, gitSourceContext string) {
	component := &appstudiov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		Spec: appstudiov1alpha1.ComponentSpec{
			ComponentName: resourceKey.Name,
			Application:   application,
			Source: appstudiov1alpha1.ComponentSource{
				ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
					GitSource: &appstudiov1alpha1.GitSource{
						URL:      gitURL,
						Revision: gitRevision,
						Context:  gitSourceContext,
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, component)).Should(Succeed())
	getComponent(resourceKey)
}

func getComponent(resourceKey types.NamespacedName) *appstudiov1alpha1.Component {
	component := &appstudiov1alpha1.Component{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, resourceKey, component); err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())
	return component
}

// deleteComponent deletes the specified component resource and verifies it was properly deleted
func deleteComponent(resourceKey types.NamespacedName) {
	component := &appstudiov1alpha1.Component{}

	// Check if the component exists
	if err := k8sClient.Get(ctx, resourceKey, component); k8sErrors.IsNotFound(err) {
		return
	}

	// Delete
	Eventually(func() error {
		if err := k8sClient.Get(ctx, resourceKey, component); err != nil {
			return err
		}
		return k8sClient.Delete(ctx, component)
	}, timeout, interval).Should(Succeed())

	// Wait for delete to finish
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, component))
	}, timeout, interval).Should(BeTrue())
}

func disableComponentMintmaker(resourceKey types.NamespacedName) {
	component := &appstudiov1alpha1.Component{}
	Expect(k8sClient.Get(ctx, resourceKey, component)).Should(Succeed())

	if component.Annotations == nil {
		component.Annotations = make(map[string]string)
	}

	component.Annotations[MintMakerDisabledAnnotationName] = "true"

	Expect(k8sClient.Update(ctx, component)).Should(Succeed())

	getComponent(resourceKey)
}

func createDependencyUpdateCheck(resourceKey types.NamespacedName, processed bool, workspaces []mmv1alpha1.WorkspaceSpec) {
	annotations := map[string]string{}
	if processed {
		annotations[MintMakerProcessedAnnotationName] = "true"
	}

	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1alpha1",
			Kind:       "DependencyUpdateCheck",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        resourceKey.Name,
			Namespace:   resourceKey.Namespace,
			Annotations: annotations,
		},
		Spec: mmv1alpha1.DependencyUpdateCheckSpec{},
	}
	if len(workspaces) > 0 {
		dependencyUpdateCheck.Spec.Workspaces = workspaces
	}

	Expect(k8sClient.Create(ctx, dependencyUpdateCheck)).Should(Succeed())
	getDependencyUpdateCheck(resourceKey)
}

func getDependencyUpdateCheck(resourceKey types.NamespacedName) *mmv1alpha1.DependencyUpdateCheck {
	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck); err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())
	return dependencyUpdateCheck
}

func deleteDependencyUpdateCheck(resourceKey types.NamespacedName) {
	dependencyUpdateCheck := &mmv1alpha1.DependencyUpdateCheck{}
	if err := k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, dependencyUpdateCheck); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, dependencyUpdateCheck))
	}, timeout, interval).Should(BeTrue())
}

func listPipelineRuns(namespace string) []tektonv1.PipelineRun {
	pipelineruns := &tektonv1.PipelineRunList{}

	err := k8sClient.List(ctx, pipelineruns, client.InNamespace(namespace))
	Expect(err).ToNot(HaveOccurred())
	return pipelineruns.Items
}

func listJobs(namespace string) []batch.Job {
	jobs := &batch.JobList{}

	err := k8sClient.List(ctx, jobs, client.InNamespace(namespace))
	Expect(err).ToNot(HaveOccurred())
	return jobs.Items
}

func deletePipelineRuns(namespace string) {
	err := k8sClient.DeleteAllOf(ctx, &tektonv1.PipelineRun{}, client.InNamespace(namespace), client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		return len(listPipelineRuns(namespace)) == 0
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

func deleteJobs(namespace string) {
	err := k8sClient.DeleteAllOf(ctx, &batch.Job{}, client.InNamespace(namespace), client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		return len(listJobs(namespace)) == 0
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

func generateInstallation(repositories []*gh.Repository) github.ApplicationInstallation {
	return github.ApplicationInstallation{
		ID:           int64(rand.Intn(100)),
		Token:        RandomString(30),
		Repositories: repositories,
	}
}

func generateRepository(repoURL string) *gh.Repository {
	repoURLParts := strings.Split(repoURL, "/")
	return &gh.Repository{
		HTMLURL:  &repoURL,
		FullName: gh.String(fmt.Sprintf("%s/%s", repoURLParts[3], repoURLParts[4])),
	}
}

func generateRepositories(repoURL []string) []*gh.Repository {
	repositories := []*gh.Repository{}
	for _, repo := range repoURL {
		repositories = append(repositories, generateRepository(repo))
	}
	return repositories
}
