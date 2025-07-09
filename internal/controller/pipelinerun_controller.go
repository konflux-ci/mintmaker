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
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"

	github "github.com/konflux-ci/mintmaker/internal/pkg/component/github"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
	mintmakermetrics "github.com/konflux-ci/mintmaker/internal/pkg/metrics"
)

var (
	MaxSimultaneousPipelineRuns      = 40
	MintMakerGitPlatformLabel        = "mintmaker.appstudio.redhat.com/git-platform"
	MintMakerComponentNameLabel      = "mintmaker.appstudio.redhat.com/component"
	MintMakerComponentNamespaceLabel = "mintmaker.appstudio.redhat.com/namespace"
)

// PipelineRunReconciler reconciles a PipelineRun object
type PipelineRunReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// updatePipelineRunState updates the status of a PipelineRun
func (r *PipelineRunReconciler) updatePipelineRunState(
	ctx context.Context,
	pipelineRun tektonv1.PipelineRun,
	status tektonv1.PipelineRunSpecStatus,
	errmsg string,
) error {
	log := ctrllog.FromContext(ctx)
	originalPipelineRun := pipelineRun.DeepCopy()
	pipelineRun.Spec.Status = status

	// If pipelinerun is to be cancelled, add reason with the error message
	if status == tektonv1.PipelineRunSpecStatusCancelled {
		pipelineRun.Status.MarkFailed(string(tektonv1.PipelineRunReasonCancelled), "%s", errmsg)
	}

	patch := client.MergeFrom(originalPipelineRun)
	if err := r.Client.Patch(ctx, &pipelineRun, patch); err != nil {
		log.Error(err, "unable to update pipelinerun status", "pipelinerun", pipelineRun.Name)
		return err
	}
	return nil
}

// handleGitHubPipelineRun processes a GitHub-specific PipelineRun by updating its token if needed
func (r *PipelineRunReconciler) handleGitHubPipelineRun(ctx context.Context, plr tektonv1.PipelineRun) error {
	componentName := plr.Labels[MintMakerComponentNameLabel]
	componentNamespace := plr.Labels[MintMakerComponentNamespaceLabel]

	// Get the component
	var component appstudiov1alpha1.Component
	componentKey := types.NamespacedName{Namespace: componentNamespace, Name: componentName}
	if err := r.Client.Get(ctx, componentKey, &component); err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}

	// Create GitHub component
	ghComponent, err := github.NewComponent(&component, r.Client, ctx)
	if err != nil {
		return fmt.Errorf("failed to create GitHub component: %w", err)
	}

	// Get token (this will renew it if needed)
	token, err := (*ghComponent).GetToken()
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}
	tokenBytes := []byte(token)

	// Get the secret to update
	appSecret := corev1.Secret{}
	appSecretKey := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plr.Name}
	if err := r.Client.Get(ctx, appSecretKey, &appSecret); err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Update the token in the secret if it has changed
	if !bytes.Equal(appSecret.Data["renovate-token"], tokenBytes) {
		originalAppSecret := appSecret.DeepCopy()
		appSecret.Data["renovate-token"] = tokenBytes

		secretPatch := client.MergeFrom(originalAppSecret)
		if err := r.Client.Patch(ctx, &appSecret, secretPatch); err != nil {
			return fmt.Errorf("failed to update secret with new token: %w", err)
		}
	}

	return nil
}

// startPipelineRun starts a pending PipelineRun by removing its pending status
// Returns true if successfully started, false otherwise
func (r *PipelineRunReconciler) startPipelineRun(ctx context.Context, plr tektonv1.PipelineRun) bool {
	log := ctrllog.FromContext(ctx)

	// If this is a GitHub PipelineRun, handle token refresh
	if plr.Labels[MintMakerGitPlatformLabel] == "github" {
		if err := r.handleGitHubPipelineRun(ctx, plr); err != nil {
			log.Error(err, "failed to handle GitHub PipelineRun", "name", plr.Name)
			_ = r.updatePipelineRunState(ctx, plr, tektonv1.PipelineRunSpecStatusCancelled, err.Error())
			return false
		}
	}

	// Start the PipelineRun by removing the pending status
	log.Info("starting PipelineRun", "name", plr.Name)
	if err := r.updatePipelineRunState(ctx, plr, "", ""); err != nil {
		log.Error(err, "failed to start PipelineRun", "name", plr.Name)
		return false
	}

	return true
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")
	ctx = ctrllog.IntoContext(ctx, log)

	// Only process events in the MintMaker namespace
	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	// Get all PipelineRuns in the namespace
	var pipelineRunList tektonv1.PipelineRunList
	if err := r.Client.List(ctx, &pipelineRunList, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "unable to list PipelineRuns")
		return ctrl.Result{}, err
	}

	// Count running PipelineRuns
	runningCount := 0
	var pendingRuns []tektonv1.PipelineRun

	for i := range pipelineRunList.Items {
		run := pipelineRunList.Items[i]

		// Count running PipelineRuns
		if len(run.Status.Conditions) > 0 &&
			run.Status.Conditions[0].Status == corev1.ConditionUnknown &&
			(run.Status.Conditions[0].Reason == tektonv1.PipelineRunReasonRunning.String() ||
				run.Status.Conditions[0].Reason == tektonv1.PipelineRunReasonStarted.String()) {
			runningCount++
		}

		// Collect pending PipelineRuns
		if run.IsPending() {
			pendingRuns = append(pendingRuns, run)
		}
	}

	// Sort pending runs by creation timestamp (oldest first)
	sort.Slice(pendingRuns, func(i, j int) bool {
		return pendingRuns[i].CreationTimestamp.Before(&pendingRuns[j].CreationTimestamp)
	})

	// Calculate how many more runs we can start
	availableSlots := MaxSimultaneousPipelineRuns - runningCount

	// Start as many pending runs as possible, up to the maximum allowed
	if availableSlots > 0 {
		started := 0
		for i := 0; i < len(pendingRuns) && started < availableSlots; i++ {
			if r.startPipelineRun(ctx, pendingRuns[i]) {
				started++
				mintmakermetrics.CountScheduledRunSuccess()
			} else {
				mintmakermetrics.CountScheduledRunFailure()
			}
		}
		log.Info("started PipelineRuns", "count", started)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1.PipelineRun{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				if pipelineRun, ok := createEvent.Object.(*tektonv1.PipelineRun); ok {
					return pipelineRun.Spec.Status == tektonv1.PipelineRunSpecStatusPending
				}
				return false
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				if oldPipelineRun, ok := updateEvent.ObjectOld.(*tektonv1.PipelineRun); ok {
					if newPipelineRun, ok := updateEvent.ObjectNew.(*tektonv1.PipelineRun); ok {
						if oldPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() &&
							!newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() {
							if newPipelineRun.Namespace == MintMakerNamespaceName &&
								newPipelineRun.Status.CompletionTime != nil {
								log := ctrl.Log.WithName("PipelineRunController")
								log.Info(
									fmt.Sprintf("PipelineRun finished: %s", newPipelineRun.Name),
									"completionTime",
									newPipelineRun.Status.CompletionTime.Format(time.RFC3339),
									"success",
									newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsTrue(),
									"reason",
									newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).GetReason(),
								)
							}
							return true
						}
					}
				}
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}
