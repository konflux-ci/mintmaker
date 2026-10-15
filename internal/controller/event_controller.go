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
	"errors"
	"regexp"
	"strings"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	konfluxv1alpha1 "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	component "github.com/konflux-ci/mintmaker/internal/component"
	mmconst "github.com/konflux-ci/mintmaker/internal/constant"
	"github.com/konflux-ci/mintmaker/internal/utils"
)

// EventReconciler reconciles a Event object
type EventReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NewGitComponent component.GitComponentFactory
}

// markEventAsProcessed adds an annotation to the event indicating it has been processed
func (r *EventReconciler) markEventAsProcessed(ctx context.Context, event *corev1.Event) error {
	patchEvent := event.DeepCopy()

	if patchEvent.Annotations == nil {
		patchEvent.Annotations = make(map[string]string)
	}

	original := patchEvent.DeepCopy()
	patchEvent.Annotations[mmconst.MintMakerProcessedAnnotationNameLegacy] = "true"
	patchEvent.Annotations[mmconst.MintMakerProcessedAnnotationName] = "true"

	patch := client.MergeFrom(original)

	if err := r.Patch(ctx, patchEvent, patch); err != nil {
		return err
	}
	return nil
}

// Reports whether gitComp's host and repository match the mintmaker git-source labels on the pod.
func gitSourceMatchesPod(pod *corev1.Pod, gitComp component.GitComponent) bool {
	podHost := utils.GetLabel(pod, mmconst.MintMakerGitHostLabel, mmconst.MintMakerGitHostLabelLegacy)
	podRepo := utils.GetLabel(pod, mmconst.MintMakerRepositoryLabel, mmconst.MintMakerRepositoryLabelLegacy)

	if gitComp.GetHost() != podHost {
		return false
	}
	if utils.NormalizeLabelValue(gitComp.GetRepository()) != podRepo {
		return false
	}
	return true
}

// Finds the corresponding Component for the given Pod and creates a GitComponent for it.
func (r *EventReconciler) getGitComponent(ctx context.Context, pod *corev1.Pod) (component.GitComponent, error) {
	componentName := utils.GetLabel(pod, mmconst.MintMakerComponentNameLabel, mmconst.MintMakerComponentNameLabelLegacy)
	componentNamespace := utils.GetLabel(pod, mmconst.MintMakerComponentNamespaceLabel, mmconst.MintMakerComponentNamespaceLabelLegacy)

	// Prefer the konflux Component when it exists and matches the pod's git-source labels.
	var konfluxComp konfluxv1alpha1.Component
	if err := r.Get(ctx, client.ObjectKey{Namespace: componentNamespace, Name: componentName}, &konfluxComp); err != nil {
		if err := ignoreMissingCRD(err); err != nil {
			return nil, err
		}
	} else {
		gitComp, err := r.NewGitComponent(ctx, &konfluxComp, r.Client)
		if err != nil {
			return nil, err
		}
		if gitSourceMatchesPod(pod, gitComp) {
			return gitComp, nil
		}
	}

	// Fallback to legacy Component if konflux was missing or mismatched.
	// TODO: remove this when appstudio Components are decommissioned.
	var appstudioComp appstudiov1alpha1.Component
	if err := r.Get(ctx, client.ObjectKey{Namespace: componentNamespace, Name: componentName}, &appstudioComp); err != nil {
		return nil, err
	}
	converted := component.ToKonfluxComponent(&appstudioComp)
	gitComp, err := r.NewGitComponent(ctx, converted, r.Client)
	if err != nil {
		return nil, err
	}
	if gitSourceMatchesPod(pod, gitComp) {
		return gitComp, nil
	}

	return nil, errors.New("git source does not match pod labels")
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;update
// +kubebuilder:rbac:groups=appstudio.redhat.com,resources=components,verbs=get
// +kubebuilder:rbac:groups=konflux-ci.dev,resources=components,verbs=get
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;patch
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns/status,verbs=patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("EventController")
	ctx = ctrllog.IntoContext(ctx, log)

	var evt corev1.Event
	var pod corev1.Pod
	var errMessage string
	defer func() {
		if evt.Name != "" {
			if err := r.markEventAsProcessed(ctx, &evt); err != nil {
				log.Error(err, "failed to mark event as processed", "event", evt.Name)
			}
		}

		// If any error happens and we can't generate the token for the pod,
		// we should cancel the corresponding pipelinerun, otherwise it will
		// remains in running state, waiting for the secret to be ready until
		// timeout.
		if pod.Name != "" && errMessage != "" {
			plrName, ok := pod.Labels["tekton.dev/pipelineRun"]
			if !ok {
				return
			}

			var plr tektonv1.PipelineRun
			if err := r.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: plrName}, &plr); err != nil {
				if apierrors.IsNotFound(err) {
					// The PipelineRun is gone, we can't update it.
					return
				}
				log.Error(err, "unable to get corresponding pipelinerun for cancellation", "pod", pod.Name)
				// Cannot proceed if we can't get the PipelineRun.
				return
			}

			// Cancel the PipelineRun
			original := plr.DeepCopy()
			plr.Spec.Status = tektonv1.PipelineRunSpecStatusCancelled
			plr.Status.MarkFailed(string(tektonv1.PipelineRunReasonCancelled), "%s", errMessage)
			patch := client.MergeFrom(original)
			err := r.Patch(ctx, &plr, patch)
			if err != nil {
				log.Error(err, "unable to cancel pipelinerun")
			} else {
				log.Info("pipelinerun is cancelled", "reason", errMessage)
			}
		}
	}()

	if err := r.Get(ctx, req.NamespacedName, &evt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check that the involved object is a Pod
	if evt.InvolvedObject.Kind != "Pod" {
		return ctrl.Result{}, nil
	}

	// Extract volume name from the event message
	// When the event is triggered by missing Renovate token, the message is in this format:
	// MountVolume.SetUp failed for volume "secret-renovate-07110345-e8d4c7bf" : references non-existent secret key: renovate-token
	msgRegex := regexp.MustCompile(`volume "([^"]+)".*references non-existent secret key: renovate-token`)
	matches := msgRegex.FindStringSubmatch(evt.Message)

	if len(matches) < 2 {
		// This is not an event we're monitoring
		return ctrl.Result{}, nil
	}
	volumeName := matches[1]

	// Get the pod details from the involved object
	podName := evt.InvolvedObject.Name
	podNamespace := evt.InvolvedObject.Namespace

	// Get the actual corresponding Pod object for this event
	if err := r.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: podName}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod has gone, we can't proceed
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log = log.WithValues(
		"component", utils.GetLabel(&pod, mmconst.MintMakerComponentNameLabel, mmconst.MintMakerComponentNameLabelLegacy),
		"componentNamespace", utils.GetLabel(&pod, mmconst.MintMakerComponentNamespaceLabel, mmconst.MintMakerComponentNamespaceLabelLegacy),
		"pipelineRun", pod.Labels["tekton.dev/pipelineRun"],
		"repository", strings.ReplaceAll(
			utils.GetLabel(&pod, mmconst.MintMakerRepositoryLabel, mmconst.MintMakerRepositoryLabelLegacy),
			"_", "/",
		),
		"gitHost", utils.GetLabel(&pod, mmconst.MintMakerGitHostLabel, mmconst.MintMakerGitHostLabelLegacy),
	)
	ctx = ctrllog.IntoContext(ctx, log)
	// Find the corresponding secret
	var secretName string
	for _, volume := range pod.Spec.Volumes {
		if volume.Name == volumeName && volume.Secret != nil {
			secretName = volume.Secret.SecretName
			break
		}
	}
	if secretName == "" {
		// Volume doesn't have a corresponding secret, ignore it
		return ctrl.Result{}, nil
	}

	// Get the secret
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: secretName}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret doesn't exist, in theory this should not happen unless someone
			// deleted the secret by manual, anyway we will ignore this
			return ctrl.Result{}, nil
		}
		errMessage = err.Error()
		return ctrl.Result{}, err
	}

	// Add the missing `renovate-token` key
	if _, hasKey := secret.Data["renovate-token"]; !hasKey {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		gitComp, err := r.getGitComponent(ctx, &pod)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Component has gone, we can't proceed
				return ctrl.Result{}, nil
			}
			errMessage = err.Error()
			return ctrl.Result{}, err
		}

		// When this is a GitHub component, it also refreshes token if needed
		token, err := gitComp.GetToken()
		if err != nil {
			log.Error(err, "failed to generate token for component")
			errMessage = err.Error()
			// Do not requeue, the error is probably caused by Konflux GitHub
			// installation issue, which retry won't help
			return ctrl.Result{}, nil
		}

		// Add the missing Renovate token
		log.Info("updating renovate token in secret", "secret", secretName)
		secret.Data["renovate-token"] = []byte(token)

		// Update the secret
		if err := r.Update(ctx, &secret); err != nil {
			log.Error(err, "failed to update renovate token in secret", "secret", secretName)
			errMessage = err.Error()
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// We only react to Create events for Event in mintmaker namespace.
	// Namespace filtering is handled by the manager's cache configuration.
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Event{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				evt, ok := e.Object.(*corev1.Event)
				if !ok {
					return false
				}
				if evt.Reason != "FailedMount" {
					return false
				}
				if utils.HasAnnotation(evt, mmconst.MintMakerProcessedAnnotationName, mmconst.MintMakerProcessedAnnotationNameLegacy) {
					return false
				}
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}
