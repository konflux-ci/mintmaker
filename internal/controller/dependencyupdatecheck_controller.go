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
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/konflux-ci/mintmaker/internal/component"
	"github.com/konflux-ci/mintmaker/internal/config"
	. "github.com/konflux-ci/mintmaker/internal/constant"
	mintmakermetrics "github.com/konflux-ci/mintmaker/internal/metrics"
	"github.com/konflux-ci/mintmaker/internal/tekton"
	"github.com/konflux-ci/mintmaker/internal/utils"
)

const InternalSecretLabelName = "appstudio.redhat.com/internal"

// DependencyUpdateCheckReconciler reconciles a DependencyUpdateCheck object
type DependencyUpdateCheckReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
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

// Returns a merged docker config that contains all image registry secrets
// linked to the component's build-pipeline ServiceAccount.
func (r *DependencyUpdateCheckReconciler) getMergedDockerConfigJson(ctx context.Context, comp component.GitComponent) ([]byte, error) {
	log := ctrllog.FromContext(ctx).WithName("getMergedDockerConfigJson")

	componentNamespace := comp.GetNamespace()
	serviceAccountName := "build-pipeline-" + comp.GetName()
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: componentNamespace, Name: serviceAccountName}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			log.Info("service account not found in component namespace", "service-account", serviceAccountName)
			return nil, nil
		}
		log.Error(err, "unable to get service account in component namespace", "service-account", serviceAccountName)
		return nil, err
	}

	mergedAuths := make(map[string]interface{})
	for _, secretRef := range serviceAccount.Secrets {
		var secret corev1.Secret
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: componentNamespace, Name: secretRef.Name}, &secret); err != nil {
			if errors.IsNotFound(err) {
				log.Info("secret not found in component namespace", "secret", secretRef.Name)
				continue
			}
			log.Error(err, "unable to get secret in component namespace", "secret", secretRef.Name)
			return nil, err
		}

		if secret.Type != corev1.SecretTypeDockerConfigJson {
			continue // Skip Secrets with type other than DockerConfigJson
		}

		if value, exists := secret.Labels[InternalSecretLabelName]; exists && value == "true" {
			continue // Skip Secrets labeled as internal
		}

		data, exists := secret.Data[corev1.DockerConfigJsonKey]
		if !exists {
			log.Info("skipping secret with missing section", "secret", secret.Name, "section", corev1.DockerConfigJsonKey)
			continue
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

	if len(mergedAuths) == 0 {
		log.Info("merged auths empty for component")
		return nil, nil
	}

	mergedDockerConfig := map[string]interface{}{
		"auths": mergedAuths,
	}
	mergedDockerConfigJson, err := json.Marshal(mergedDockerConfig)
	if err != nil {
		return nil, err
	}
	return mergedDockerConfigJson, nil
}

// createPipelineRun creates and returns a new PipelineRun
func (r *DependencyUpdateCheckReconciler) createPipelineRun(ctx context.Context, name string, comp component.GitComponent, currentBranch string, kiteSecretName string) (*tektonv1.PipelineRun, error) {

	log := ctrllog.FromContext(ctx).WithName("createPipelineRun")

	var resources []client.Object
	defer func() {
		if len(resources) > 0 {
			for _, resource := range resources {
				// Ignore error
				r.Client.Delete(ctx, resource)
			}
		}
	}()

	// Secret for Renovate token (repository access token) and image registry credentials
	renovateSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{},
	}

	// For GitHub repositories, we intentionally do not set the "renovate-token"
	// key. GitHub tokens generated from the Konflux GitHub application have a
	// maximum lifespan of 1 hour. Instead of generating a token that might expire
	// before the pipelinerun starts, we wait for events with "FailedMount" reason,
	// which happens when pod try to mount the secret but can't find the key in
	// secret. Then we populate the token in the event controller at that time to
	// ensure it's valid for the pipelinerun execution.
	if comp.GetPlatform() != "github" {
		renovateToken, err := comp.GetToken()
		if err != nil {
			return nil, err
		}
		renovateSecret.StringData["renovate-token"] = renovateToken
	}

	// Add a merged docker config to the renovateSecret
	mergedDockerConfigJson, err := r.getMergedDockerConfigJson(ctx, comp)
	if err != nil {
		log.Error(err, "failed to get a merged docker config for component")
		return nil, err
	}
	if len(mergedDockerConfigJson) != 0 {
		renovateSecret.Data = map[string][]byte{corev1.DockerConfigJsonKey: mergedDockerConfigJson}
	}

	if err := r.Client.Create(ctx, renovateSecret); err != nil {
		return nil, err
	}
	resources = append(resources, renovateSecret)

	renovateConfig, err := comp.GetRenovateConfig(renovateSecret, currentBranch)
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

	// Create Secret for RPM activation key to access RPMs that require subscription
	activationKey, org, rpmKeyErr := comp.GetRPMActivationKey(ctx, r.Client)
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
			"mintmaker.appstudio.redhat.com/branch":       utils.NormalizeLabelValue(currentBranch),
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

	if _, exists := renovateSecret.Data[corev1.DockerConfigJsonKey]; exists {
		secretItems := []corev1.KeyToPath{
			{
				Key:  corev1.DockerConfigJsonKey,
				Path: "config.json",
			},
		}
		secretOpts := tekton.NewMountOptions().WithTaskName("build").WithStepNames([]string{"renovate"}).WithReadOnly(true)
		builder.WithSecret(renovateSecret.ObjectMeta.Name, "/home/renovate/.docker", secretItems, secretOpts)
	}

	// Add Kite integration if enabled AND token is available
	if kiteSecretName != "" {
		builder.WithKiteIntegration(config.Get().Kite.APIURL)

		tokenSecretItems := []corev1.KeyToPath{
			{
				Key:  "token",
				Path: "token",
			},
		}
		tokenSecretOpts := tekton.NewMountOptions().
			WithTaskName("build").
			WithStepNames([]string{"log-analyzer"}).
			WithReadOnly(true)
		builder.WithSecret(kiteSecretName, "/var/run/secrets/kite", tokenSecretItems, tokenSecretOpts)
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

// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=dependencyupdatechecks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=dependencyupdatechecks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=dependencyupdatechecks/finalizers,verbs=update
// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=anyuid,verbs=use

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DependencyUpdateCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)

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

	// Record metrics for DependencyUpdateCheck creation
	mintmakermetrics.RecordDependencyUpdateCheckCreation(dependencyupdatecheck.Namespace, dependencyupdatecheck.Name)
	log.Info("Recorded DependencyUpdateCheck creation metrics", "namespace", dependencyupdatecheck.Namespace, "name", dependencyupdatecheck.Name)

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
		gatheredComponents, err = getFilteredComponents(ctx, dependencyupdatecheck.Spec.Namespaces, r.Client)
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

	// Check for token Secret if Kite integration is enabled (token needed for Kite API requests)
	// kiteSecretName is only set if Kite integration is enabled and the token secret is found
	kiteSecretName := ""
	if cfg := config.Get(); cfg.Kite.Enabled {
		secretList := &corev1.SecretList{}
		err := r.Client.List(ctx, secretList,
			client.InNamespace(MintMakerNamespaceName),
			client.MatchingLabels{KiteTokenSecretLabel: "true"},
		)
		if err != nil {
			log.Error(err, "Kite token secret lookup failed - skipping Kite integration")
		} else if len(secretList.Items) == 0 {
			log.Info("Kite token secret not found - skipping Kite integration")
		} else {
			kiteSecretName = secretList.Items[0].Name
			log.Info("Kite token secret found - using it", "secretName", kiteSecretName)
		}
	}

	// Track components for which we already created a PipelineRun
	processedComponents := make([]string, 0)

	timestamp := time.Now().UTC().Format("01021504") // MMDDhhmm, from Go's time formatting reference date "20060102150405"
	for _, appstudioComponent := range componentList {
		compLog := log.WithValues("component", appstudioComponent.Name,
			"componentNamespace", appstudioComponent.Namespace)
		ctx = ctrllog.IntoContext(ctx, compLog)

		comp, err := component.NewGitComponent(ctx, &appstudioComponent, r.Client)
		if err != nil {
			compLog.Error(err, "failed to handle component")
			continue
		}

		versions, err := comp.GetCompVersionsBranches()
		if err != nil {
			compLog.Info("couldn't find versions which are branches for component", "component", appstudioComponent.Name, "err", err)
			continue
		}

		host := comp.GetHost()
		repository := comp.GetRepository()
		for _, versionName := range versions {
			// We need to create only one PipelineRun for a combination
			// of repository+branch. We cannot use repository only,
			// because the branch is used in Renovate's baseBranch config option.
			branchLog := compLog.WithValues("repository", repository,
				"branch", versionName,
				"gitHost", host)
			ctx = ctrllog.IntoContext(ctx, branchLog)

			key := fmt.Sprintf("%s/%s@%s", host, repository, versionName)
			if slices.Contains(processedComponents, key) {
				// PipelineRun has already been created for this repo-branch
				branchLog.Info("PipelineRun has been created for this component-key", "component-key", key)
				continue
			} else {
				processedComponents = append(processedComponents, key)
			}

			plrName := fmt.Sprintf("renovate-%s-%s", timestamp, utils.RandomString(8))
			pipelinerun, err := r.createPipelineRun(ctx, plrName, comp, versionName, kiteSecretName)
			if err != nil {
				branchLog.Error(err, "failed to create PipelineRun")
				mintmakermetrics.CountScheduledRunFailure()
			} else {
				branchLog.Info("created PipelineRun", "pipelineRun", pipelinerun.Name)
				mintmakermetrics.CountScheduledRunSuccess()
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DependencyUpdateCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// We only react to Create events for DependencyUpdateCheck in mintmaker namespace.
	// Namespace filtering is handled by the manager's cache configuration.
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1alpha1.DependencyUpdateCheck{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
