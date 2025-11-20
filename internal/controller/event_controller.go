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
	"regexp"
	"strings"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	component "github.com/konflux-ci/mintmaker/internal/pkg/component"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
)

// EventReconciler reconciles a Event object
type EventReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

// markEventAsProcessed adds an annotation to the event indicating it has been processed
func (r *EventReconciler) markEventAsProcessed(ctx context.Context, event *corev1.Event) error {
	patchEvent := event.DeepCopy()

	if patchEvent.Annotations == nil {
		patchEvent.Annotations = make(map[string]string)
	}

	original := patchEvent.DeepCopy()
	patchEvent.Annotations["mintmaker.appstudio.redhat.com/processed"] = "true"

	patch := client.MergeFrom(original)

	if err := r.Patch(ctx, patchEvent, patch); err != nil {
		return err
	}
	return nil
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events/finalizers,verbs=update
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns;pipelineruns/status,verbs=get;list;watch;create;update;patch

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
			if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: plrName}, &plr); err != nil {
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

	if err := r.APIReader.Get(ctx, req.NamespacedName, &evt); err != nil {
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
	if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: podName}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod has gone, we can't proceed
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log = log.WithValues(
		"component", pod.Labels[MintMakerComponentNameLabel],
		"componentNamespace", pod.Labels[MintMakerComponentNamespaceLabel],
		"pipelineRun", pod.Labels["tekton.dev/pipelineRun"],
		"repository", strings.ReplaceAll(pod.Labels["mintmaker.appstudio.redhat.com/repository"], "_", "/"),
		"gitHost", pod.Labels["mintmaker.appstudio.redhat.com/git-host"],
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
	if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: secretName}, &secret); err != nil {
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

		componentName := pod.Labels[MintMakerComponentNameLabel]
		componentNamespace := pod.Labels[MintMakerComponentNamespaceLabel]

		// Get the component
		var comp appstudiov1alpha1.Component
		if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: componentNamespace, Name: componentName}, &comp); err != nil {
			if apierrors.IsNotFound(err) {
				// Component has gone, we can't proceed
				return ctrl.Result{}, nil
			}
			errMessage = err.Error()
			return ctrl.Result{}, err
		}

		// Create GitComponent from Component
		gitComp, err := component.NewGitComponent(ctx, &comp, r.Client)
		if err != nil {
			errMessage = err.Error()
			// Do not requeue, the error is not related to the cluster issues
			return ctrl.Result{}, nil
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Event{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetNamespace() == MintMakerNamespaceName
		}))).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				if e.Object.GetNamespace() != MintMakerNamespaceName {
					return false
				}
				evt, ok := e.Object.(*corev1.Event)
				if !ok {
					return false
				}
				if evt.Reason != "FailedMount" {
					return false
				}
				if _, exists := evt.Annotations["mintmaker.appstudio.redhat.com/processed"]; exists {
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
