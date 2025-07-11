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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
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
	Scheme *runtime.Scheme
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Event object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *EventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("EventController")
	ctx = ctrllog.IntoContext(ctx, log)

	var evt corev1.Event
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
		if err := r.markEventAsProcessed(ctx, &evt); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	volumeName := matches[1]

	// Get the pod details from the involved object
	podName := evt.InvolvedObject.Name
	podNamespace := evt.InvolvedObject.Namespace

	// Get the actual corresponding Pod object for this event
	var pod corev1.Pod
	if err := r.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: podName}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// This is an event related to an old Pod which doesn't exist anymore
			if err := r.markEventAsProcessed(ctx, &evt); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

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
		if err := r.markEventAsProcessed(ctx, &evt); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Get the secret
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: secretName}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret doesn't exist, in theory this should not happen unless someone
			// deleted the secret by manual, anyway we will ignore this
			if err := r.markEventAsProcessed(ctx, &evt); err != nil {
				return ctrl.Result{}, err
			}
		}
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
		if err := r.Get(ctx, client.ObjectKey{Namespace: componentNamespace, Name: componentName}, &comp); err != nil {
			return ctrl.Result{}, err
		}

		// Create GitHub component
		gitComp, err := component.NewGitComponent(&comp, r.Client, ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		// When this is a GitHub component, it also refreshs token if needed
		token, err := gitComp.GetToken()
		if err != nil {
			return ctrl.Result{}, err
		}

		// Add the missing key
		log.Info("updating renovate token in secret", "secret", secretName)
		secret.Data["renovate-token"] = []byte(token)

		// Update the secret
		if err := r.Update(ctx, &secret); err != nil {
			log.Error(err, "failed to update renovate token in secret", secret, secretName)
			return ctrl.Result{}, err
		}
	}

	if err := r.markEventAsProcessed(ctx, &evt); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Event{}).
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
