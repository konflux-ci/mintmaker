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
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/pkg/component"
	"github.com/konflux-ci/mintmaker/internal/pkg/config"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
	"github.com/konflux-ci/mintmaker/internal/pkg/tekton"
	"github.com/konflux-ci/mintmaker/internal/pkg/utils"
)

// DependencyUpdateCheckReconciler reconciles a DependencyUpdateCheck object
type DependencyUpdateCheckReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Config config.ControllerConfig
}

func NewDependencyUpdateCheckReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder) *DependencyUpdateCheckReconciler {
	return &DependencyUpdateCheckReconciler{
		Client: client,
		Scheme: scheme,
	}
}

// getCAConfigMap returns the first ConfigMap found in mintmaker namespace
// that has the label 'config.openshift.io/inject-trusted-cabundle: "true"'.
// If no such ConfigMap is found, it returns nil.
func (r *DependencyUpdateCheckReconciler) getCAConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	configMapList := &corev1.ConfigMapList{}
	labelSelector := client.MatchingLabels{"config.openshift.io/inject-trusted-cabundle": "true"}
	listOptions := []client.ListOption{
		client.InNamespace(MintMakerNamespaceName),
		labelSelector,
	}
	err := r.Client.List(ctx, configMapList, listOptions...)
	if err != nil {
		return nil, err
	}

	if len(configMapList.Items) > 0 {
		// Just return the configmap
		return &configMapList.Items[0], nil
	}

	return nil, nil
}

// Create a secret that merges all secret with the label:
// mintmaker.appstudio.redhat.com/secret-type: registry
// and return the new secret
func (r *DependencyUpdateCheckReconciler) createMergedPullSecret(ctx context.Context) (*corev1.Secret, error) {
	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)

	secretList := &corev1.SecretList{}
	labelSelector := client.MatchingLabels{"mintmaker.appstudio.redhat.com/secret-type": "registry"}
	listOptions := []client.ListOption{
		client.InNamespace(MintMakerNamespaceName),
		labelSelector,
	}

	err := r.Client.List(ctx, secretList, listOptions...)

	if err != nil {
		return nil, err
	}

	if len(secretList.Items) == 0 {
		// No secrets to merge
		return nil, nil
	}

	log.Info(fmt.Sprintf("Found %d secrets to merge", len(secretList.Items)))

	mergedAuths := make(map[string]interface{})
	for _, secret := range secretList.Items {
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			data, exists := secret.Data[".dockerconfigjson"]
			if !exists {
				// No .dockerconfigjson section
				log.Info("Found secret without .dockerconfigjson section")
				return nil, nil
			}

			var dockerConfig map[string]interface{}
			if err := json.Unmarshal(data, &dockerConfig); err != nil {
				return nil, err
			}

			auths, exists := dockerConfig["auths"].(map[string]interface{})
			if !exists {
				continue
			}

			for registry, creds := range auths {
				mergedAuths[registry] = creds
			}
		}
	}

	mergedDockerConfig := map[string]interface{}{
		"auths": mergedAuths,
	}

	if len(mergedAuths) == 0 {
		log.Info("Merged auths empty, skipping creation of secret")
		return nil, nil
	}

	mergedConfigJson, err := json.Marshal(mergedDockerConfig)
	if err != nil {
		return nil, err
	}

	timestamp := time.Now().UTC().Format("01021504") // MMDDhhmm, from Go's time formatting reference date "20060102150405"
	name := fmt.Sprintf("renovate-image-pull-secrets-%s-%s", timestamp, utils.RandomString(5))

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(mergedConfigJson),
		},
	}

	if err := r.Client.Create(ctx, newSecret); err != nil {
		return nil, err
	}

	return newSecret, nil
}

// createPipelineRun creates and returns a new PipelineRun
func (r *DependencyUpdateCheckReconciler) createPipelineRun(name string, comp component.GitComponent, ctx context.Context, registrySecret *corev1.Secret) (*tektonv1.PipelineRun, error) {

	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)

	var resources []client.Object
	defer func() {
		if len(resources) > 0 {
			for _, resource := range resources {
				// Ignore error
				r.Client.Delete(ctx, resource)
			}
		}
	}()

	renovateConfig, err := comp.GetRenovateConfig(registrySecret)
	if err != nil {
		return nil, err
	}
	renovateJsConfig := "module.exports = " + renovateConfig
	// Create ConfigMap for Renovate global configuration
	renovateConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Data: map[string]string{
			"config.js": renovateJsConfig,
		},
	}

	if err := r.Client.Create(ctx, renovateConfigMap); err != nil {
		return nil, err
	}
	resources = append(resources, renovateConfigMap)

	// Create Secret for Renovate token (the repository access token)
	renovateToken, err := comp.GetToken()
	if err != nil {
		return nil, err
	}
	renovateSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"renovate-token": renovateToken,
		},
	}

	if err := r.Client.Create(ctx, renovateSecret); err != nil {
		return nil, err
	}
	resources = append(resources, renovateSecret)

	// Create Secret for RPM activation key to access RPMs that require subscription
	activationKey, org, rpmKeyErr := comp.GetRPMActivationKey(r.Client, ctx)
	var rpmSecret *corev1.Secret = nil
	if rpmKeyErr != nil {
		log.Info(rpmKeyErr.Error())
	} else {
		rpmSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-rpm-key",
				Namespace: MintMakerNamespaceName,
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"activationkey": activationKey,
				"org":           org,
			},
		}

		if err := r.Client.Create(ctx, rpmSecret); err != nil {
			return nil, err
		}
		resources = append(resources, rpmSecret)
	}

	// Creating the pipelineRun definition
	builder := tekton.NewPipelineRunBuilder(name, MintMakerNamespaceName).
		WithLabels(map[string]string{
			"mintmaker.appstudio.redhat.com/application":  comp.GetApplication(),
			"mintmaker.appstudio.redhat.com/component":    comp.GetName(),
			"mintmaker.appstudio.redhat.com/namespace":    comp.GetNamespace(),
			"mintmaker.appstudio.redhat.com/git-platform": comp.GetPlatform(), // (github, gitlab)
			"mintmaker.appstudio.redhat.com/git-host":     comp.GetHost(),     // github.com, gitlab.com, gitlab.other.com
			"mintmaker.appstudio.redhat.com/repository":   utils.NormalizeLabelValue(comp.GetRepository()),
		}).
		WithTimeouts(nil)
	builder.WithServiceAccount("mintmaker-controller-manager")

	cmItems := []corev1.KeyToPath{
		{
			Key:  "config.js",
			Path: "config.js",
		},
	}
	cmOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"renovate"})
	builder.WithConfigMap(name, "/etc/renovate/config", cmItems, cmOpts)

	secretItems := []corev1.KeyToPath{
		{
			Key:  "renovate-token",
			Path: "renovate-token",
		},
	}
	secretOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"renovate"})
	builder.WithSecret(name, "/etc/renovate/secret", secretItems, secretOpts)

	if rpmKeyErr == nil {
		rpmSecretItems := []corev1.KeyToPath{
			{
				Key:  "activationkey",
				Path: "rpm-activationkey",
			},
			{
				Key:  "org",
				Path: "rpm-org",
			},
		}
		rpmSecretOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"prepare-rpm-cert"})
		builder.WithSecret(name+"-rpm-key", "/etc/renovate/secret", rpmSecretItems, rpmSecretOpts)
	}

	// Check if a ConfigMap with the label `config.openshift.io/inject-trusted-cabundle: "true"` exists.
	// If such a ConfigMap is found, add a volume to the PipelineRun specification to mount this ConfigMap.
	// The volume will be mounted at '/etc/pki/ca-trust/extracted/pem' within the PipelineRun Pod.
	caConfigMap, err := r.getCAConfigMap(ctx)
	if err != nil {
		log.Error(err, "Failed to get CAConfigMap - moving on")
	}
	if caConfigMap != nil {
		caConfigMapItems := []corev1.KeyToPath{
			{
				Key:  "ca-bundle.crt",
				Path: "tls-ca-bundle.pem",
			},
		}
		caConfigMapOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"renovate"}).WithReadOnly(true)
		builder.WithConfigMap(caConfigMap.ObjectMeta.Name, "/etc/pki/ca-trust/extracted/pem", caConfigMapItems, caConfigMapOpts)
	}

	if registrySecret != nil {
		secretItems := []corev1.KeyToPath{
			{
				Key:  ".dockerconfigjson",
				Path: "config.json",
			},
		}
		secretOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"renovate"}).WithReadOnly(true)
		builder.WithSecret(registrySecret.ObjectMeta.Name, "/home/renovate/.docker", secretItems, secretOpts)
	}

	pipelineRun, err := builder.Build()
	if err != nil {
		log.Error(err, "failed to build pipeline definition")
		return nil, err
	}
	if err := r.Client.Create(ctx, pipelineRun); err != nil {
		return nil, err
	}
	resources = append(resources, pipelineRun)

	// Set ownership so all resources get deleted once the job is deleted
	// ownership for renovateSecret
	if err := controllerutil.SetOwnerReference(pipelineRun, renovateSecret, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Client.Update(ctx, renovateSecret); err != nil {
		return nil, err
	}

	// ownership for RPM secret
	if rpmKeyErr == nil {
		if err := controllerutil.SetOwnerReference(pipelineRun, rpmSecret, r.Scheme); err != nil {
			return nil, err
		}
		if err := r.Client.Update(ctx, rpmSecret); err != nil {
			return nil, err
		}
	}

	// ownership for the renovateConfigMap
	if err := controllerutil.SetOwnerReference(pipelineRun, renovateConfigMap, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Client.Update(ctx, renovateConfigMap); err != nil {
		return nil, err
	}

	resources = nil
	return pipelineRun, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DependencyUpdateCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)

	// Ignore CRs that are not from the mintmaker namespace
	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	dependencyupdatecheck := &mmv1alpha1.DependencyUpdateCheck{}
	err := r.Client.Get(ctx, req.NamespacedName, dependencyupdatecheck)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	// If the DependencyUpdateCheck has been handled before, skip it
	if value, exists := dependencyupdatecheck.Annotations[MintMakerProcessedAnnotationName]; exists && value == "true" {
		log.Info(fmt.Sprintf("DependencyUpdateCheck has been processed: %v", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	// Update the DependencyUpdateCheck to add a processed annotation
	log.Info(fmt.Sprintf("new DependencyUpdateCheck found: %v", req.NamespacedName))
	if dependencyupdatecheck.Annotations == nil {
		dependencyupdatecheck.Annotations = map[string]string{}
	}
	dependencyupdatecheck.Annotations[MintMakerProcessedAnnotationName] = "true"

	err = r.Client.Update(ctx, dependencyupdatecheck)
	if err != nil {
		log.Error(err, "failed to update DependencyUpdateCheck annotations")
		return ctrl.Result{}, nil
	}

	var gatheredComponents []appstudiov1alpha1.Component
	if len(dependencyupdatecheck.Spec.Namespaces) > 0 {
		log.Info(fmt.Sprintf("Following components are specified: %v", dependencyupdatecheck.Spec.Namespaces))
		gatheredComponents, err = getFilteredComponents(dependencyupdatecheck.Spec.Namespaces, r.Client, ctx)
		if err != nil {
			log.Error(err, "gathering filtered components has failed")
			return ctrl.Result{}, err
		}
	} else {
		allComponents := &appstudiov1alpha1.ComponentList{}
		if err := r.Client.List(ctx, allComponents, &client.ListOptions{}); err != nil {
			log.Error(err, "failed to list Components")
			return ctrl.Result{}, err
		}
		gatheredComponents = allComponents.Items
	}

	log.Info(fmt.Sprintf("%d components will be processed", len(gatheredComponents)))

	// Filter out components which have mintmaker disabled
	componentList := []appstudiov1alpha1.Component{}
	for _, component := range gatheredComponents {
		if value, exists := component.Annotations[MintMakerDisabledAnnotationName]; !exists || value != "true" {
			componentList = append(componentList, component)
		}
	}

	log.Info("found components with mintmaker disabled", "components", len(gatheredComponents)-len(componentList))
	if len(componentList) == 0 {
		return ctrl.Result{}, nil
	}

	registrySecret, _ := r.createMergedPullSecret(ctx)
	// ignore the error, image pull secret is not required for all repositories
	// and set the ownership for registrySecret
	if registrySecret != nil {
		if err := controllerutil.SetOwnerReference(dependencyupdatecheck, registrySecret, r.Scheme); err != nil {
			log.Info(fmt.Sprintf("failed to set ownership for the registry secret: %s", err.Error()))
		} else {
			if err := r.Client.Update(ctx, registrySecret); err != nil {
				log.Info(fmt.Sprintf("failed to update the registry secret: %s", err.Error()))
			}
		}
	}

	// Track components for which we already created a PipelineRun
	processedComponents := make([]string, 0)

	timestamp := time.Now().UTC().Format("01021504") // MMDDhhmm, from Go's time formatting reference date "20060102150405"
	for _, appstudioComponent := range componentList {
		comp, err := component.NewGitComponent(&appstudioComponent, r.Client, ctx)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to handle component: %s", appstudioComponent.Name))
			continue
		}

		// We need to create only one PipelineRun for a combination
		// of repository+branch. We cannot use repository only,
		// because the branch is used in Renovate's baseBranch config option.
		branch, _ := comp.GetBranch()
		key := fmt.Sprintf("%s/%s@%s", comp.GetHost(), comp.GetRepository(), branch)

		log.Info(fmt.Sprintf("check if PipelineRun has been created for %s", key))

		if slices.Contains(processedComponents, key) {
			// PipelineRun has already been created for this repo-branch
			continue
		} else {
			processedComponents = append(processedComponents, key)
		}

		log.Info(fmt.Sprintf("creating pending PipelineRun for %s", key))
		plrName := fmt.Sprintf("renovate-%s-%s", timestamp, utils.RandomString(8))
		pipelinerun, err := r.createPipelineRun(plrName, comp, ctx, registrySecret)
		if err != nil {
			log.Info(fmt.Sprintf("failed to create PipelineRun for %s: %s", appstudioComponent.Name, err.Error()))
		} else {
			log.Info(fmt.Sprintf("created PipelineRun %s", pipelinerun.Name))
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DependencyUpdateCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1alpha1.DependencyUpdateCheck{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(createEvent event.CreateEvent) bool { return true },
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return false },
			UpdateFunc:  func(updateEvent event.UpdateEvent) bool { return false },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
