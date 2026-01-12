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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"

	. "github.com/konflux-ci/mintmaker/internal/constant"
)

var testPrivateKey = "-----BEGIN PRIVATE KEY-----\n" +
	"MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC0lZNSLqX1n2aO" +
	"loffn3rBL2+3Mm/GnjkWVQz76TS3KO7RSf4Det017H80XUumx3fT26rZ+wgpN8fi" +
	"CCB2LWRlMVbQr2WxCl03l3/FmHRrbUPflbn5TWyxEUj4nST5BsIFkcXXVG2sHUuM" +
	"+ZR5sbJadLLsj2Qn6Iy7zwV/pyDf89tZMXsTvO4SBhnYvitkDg6p3ar6uXMDd3aX" +
	"Cwib8s/qJW7w7M/5duoVvtlBnsPpVjJvfmwOH2jzhzWOj0KaaS2Z+DIqnSyiU4Jd" +
	"vvUlQLp3rmvTgf6R38P3phcLwb3gdu+9BqwP73vF9R52SXxfcFNRJ3cD9GdcYdL4" +
	"/2hqqtknAgMBAAECggEAFLvfxbMpcZbdtZ1powQH73USL0c74jgkdytIvwZlpmoC" +
	"vEZHQ1WwtGefq1QoQtFAMYj/3YtJwpcZp3AmGgDYOBjUI53KipXqt16jraegJlLN" +
	"/4viEH0SmmSmVjU6G4WVHVfsDpnZBcakorO9Qm5j+1LO1a5uYiP8lKEOZpEP4JFI" +
	"1T81LuyQDgsbZlmPEuxE0GSGkHKxT+b6VmvCvkN8gjUuYkHNNwd0lbqBvOvh9rSu" +
	"2iSONFHIYJloC6e09+cDlM/DedwS3aOpaoOzq8HGeIZLCo1XoBlqWx+NyyXbDKaS" +
	"eDAldq+4npEx58E4wt5Wy/FpBHInvOLXM6RQZn2n8QKBgQDv/oL4KlPvlaCV7qtK" +
	"gBbKxLjk2EZGqAuAHaskMDTAsexyXUuzPEO2cNgPGVuiPpbYtg2LI5Wngan56f3O" +
	"7b5SUykIa0DuvMgZyp80W5pCxfHROxUL6t9gyCaOwyl9ejvocF3H40JX56WT/DZC" +
	"U0M5xiSODStYyQCzScGS4PVE7QKBgQDAoL3lqQE9obw4ipO0CKnRlU5zoitClj8L" +
	"3CtkP/uoR/lHQvJlZz0xyhV2ansZxtPBdjB5u79q8wRV8HeTCGmZKuN5tfC7ZnH4" +
	"TEaq1AjIIim2zfCJaLT1L+6jS5+MUWZPoDfWrHdGFYuPtN8FMqaojz7kI0p33uMm" +
	"lff/fjVH4wKBgA5TWu4FWM1MWTGZ9Y+U5cdkxsSiRE+jaExVeQnH9t4pwLty5jnk" +
	"twYE5mDAWr/sjISTGWvcy+oby1GnrgbUGjA/1osyG8Ykbq1bcvVlImgp+K1MoYz8" +
	"kCjuyZ5r9+YNjdXqHy73WdZ1dWTIAVUkMzcXpMb18khydyA8ntltpDZhAoGAJJil" +
	"W1OPg8kNfGR/iU24DbRjEj72HxFyautqZwJs6ly6NFq4uKEzlBkDmNrEBnKq2m98" +
	"6DPOOyBua3FjFlEb1ti6HO5/DOt6raS4LE5aWMN8z1ky4Lg+4PI5UVbVug/g8zHK" +
	"SgO8KVmAiU3grRkhZpbIaQl3ZWy4FSWa1zSAJOcCgYEA1IEZPnJDM8Hlqj/vZBRa" +
	"t2/ZKaNf3Je64KElsfvQ/SoMmOPHkfFfJK3+GqQbsjQ6Jr4MwzjDia3GcIagui3t" +
	"mvfkc6syo7pMwBIylWZVCIGtyW/CVPL8uMA+6yvgksEIeVL40DfO6cI5OH7kQp87" +
	"t8RRiu3Z5nY3spNv38kAa9s=" +
	"\n-----END PRIVATE KEY-----"

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

func createServiceAccount(resourceKey types.NamespacedName) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
	}
	if err := k8sClient.Create(ctx, sa); err != nil {
		if !k8sErrors.IsAlreadyExists(err) {
			Fail(err.Error())
		}
		deleteServiceAccount(resourceKey)
		Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
	}
	getServiceAccount(resourceKey)
}

func deleteServiceAccount(resourceKey types.NamespacedName) {
	sa := &corev1.ServiceAccount{}
	if err := k8sClient.Get(ctx, resourceKey, sa); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, sa); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, sa))
	}, timeout, interval).Should(BeTrue())
}

func getServiceAccount(resourceKey types.NamespacedName) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{}
	Eventually(func() error {
		return k8sClient.Get(ctx, resourceKey, sa)
	}, timeout, interval).Should(Succeed())
	return sa
}

func createSecret(resourceKey types.NamespacedName, secretType corev1.SecretType, data map[string][]byte, stringData map[string]string) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
		},
		Type:       secretType,
		Data:       data,
		StringData: stringData,
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

func getSecret(resourceKey types.NamespacedName) *corev1.Secret {
	secret := &corev1.Secret{}
	Eventually(func() error {
		return k8sClient.Get(ctx, resourceKey, secret)
	}, timeout, interval).Should(Succeed())
	return secret
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

func createDependencyUpdateCheck(resourceKey types.NamespacedName, processed bool, namespaces []mmv1alpha1.NamespaceSpec) {
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
	if len(namespaces) > 0 {
		dependencyUpdateCheck.Spec.Namespaces = namespaces
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

func deletePipelineRuns(namespace string) {
	err := k8sClient.DeleteAllOf(ctx, &tektonv1.PipelineRun{}, client.InNamespace(namespace), client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())
	Eventually(func() bool {
		return len(listPipelineRuns(namespace)) == 0
	}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
}

func deletePipelineRun(resourceKey types.NamespacedName) {
	pipelineRun := &tektonv1.PipelineRun{}
	if err := k8sClient.Get(ctx, resourceKey, pipelineRun); err != nil {
		if k8sErrors.IsNotFound(err) {
			return
		}
		Fail(err.Error())
	}
	if err := k8sClient.Delete(ctx, pipelineRun); err != nil {
		if !k8sErrors.IsNotFound(err) {
			Fail(err.Error())
		}
		return
	}
	Eventually(func() bool {
		return k8sErrors.IsNotFound(k8sClient.Get(ctx, resourceKey, pipelineRun))
	}, timeout, interval).Should(BeTrue())
}
