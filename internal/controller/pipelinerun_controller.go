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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	appstudiov1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/mintmaker/internal/pkg/config"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
	"github.com/konflux-ci/mintmaker/internal/pkg/kite"
)

var (
	MintMakerGitPlatformLabel        = "mintmaker.appstudio.redhat.com/git-platform"
	MintMakerComponentNameLabel      = "mintmaker.appstudio.redhat.com/component"
	MintMakerComponentNamespaceLabel = "mintmaker.appstudio.redhat.com/namespace"
)

// PipelineRunReconciler reconciles a PipelineRun object
type PipelineRunReconciler struct {
	Client     client.Client
	Scheme     *runtime.Scheme
	Config     *config.ControllerConfig
	KiteClient *kite.Client
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *PipelineRunReconciler) handlePipelinerunCompletion(ctx context.Context, pipelineRun *tektonv1.PipelineRun) error {
	condition := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil {
		return fmt.Errorf("PipelineRun condition is nil")
	}

	if r.KiteClient == nil {
		return fmt.Errorf("KITE client not available")
	}

	// Fetch the Component associated with this PipelineRun
	component := &appstudiov1alpha1.Component{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      pipelineRun.Labels[MintMakerComponentNameLabel],
		Namespace: pipelineRun.Labels[MintMakerComponentNamespaceLabel],
	}, component)

	if err != nil {
		return fmt.Errorf("failed to fetch Component %s/%s: %w",
			pipelineRun.Labels[MintMakerComponentNamespaceLabel], pipelineRun.Labels[MintMakerComponentNameLabel], err)
	}

	// Construct a unique pipeline identifier using the Git URL and revision (branch)
	pipelineIdentifier := fmt.Sprintf("%s/%s", component.Spec.Source.GitSource.URL, component.Spec.Source.GitSource.Revision)

	// Check if the PipelineRun failed or succeeded and send the appropriate webhook
	log := ctrl.Log.WithName("PipelineRunController")
	if condition.IsTrue() {
		if err := r.sendSuccessWebhook(ctx, pipelineRun, pipelineIdentifier); err != nil {
			return err
		} else {
			log.Info("Succesfully sent PipelineRun success webhook", "pipelineRun", pipelineRun.Name, "pipelineIdentifier", pipelineIdentifier)
		}
	} else {
		if err := r.sendFailureWebhook(ctx, pipelineRun, pipelineIdentifier); err != nil {
			return err
		} else {
			// Log the failure reason
			reason := pipelineRun.Status.GetCondition(apis.ConditionSucceeded).GetReason()
			log.Info("Succesfully sent PipelineRun failure webhook", "pipelineRun", pipelineRun.Name, "reason", reason, "pipelineIdentifier", pipelineIdentifier)
		}
	}
	return nil
}

func (r *PipelineRunReconciler) sendFailureWebhook(ctx context.Context, pipelineRun *tektonv1.PipelineRun, pipelineIdentifier string) error {
	// Get failure reason
	reason := pipelineRun.Status.GetCondition(apis.ConditionSucceeded).GetReason()

	payload := kite.PipelineFailurePayload{
		PipelineName:  pipelineIdentifier,
		Namespace:     pipelineRun.Labels[MintMakerComponentNamespaceLabel],
		FailureReason: reason,
		RunID:         pipelineRun.Name,
		LogsURL:       "", // Placeholder for logs URL if available
	}

	return r.KiteClient.SendPipelineFailure(ctx, payload)
}

func (r *PipelineRunReconciler) sendSuccessWebhook(ctx context.Context, pipelineRun *tektonv1.PipelineRun, pipelineIdentifier string) error {
	payload := kite.PipelineSuccessPayload{
		PipelineName: pipelineIdentifier,
		Namespace:    pipelineRun.Labels[MintMakerComponentNamespaceLabel],
	}

	return r.KiteClient.SendPipelineSuccess(ctx, payload)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1.PipelineRun{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetNamespace() == MintMakerNamespaceName
		}))).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if e.ObjectNew.GetNamespace() != MintMakerNamespaceName {
					return false
				}
				if oldPipelineRun, ok := e.ObjectOld.(*tektonv1.PipelineRun); ok {
					if newPipelineRun, ok := e.ObjectNew.(*tektonv1.PipelineRun); ok {
						if !oldPipelineRun.IsDone() && newPipelineRun.IsDone() {
							if newPipelineRun.Status.CompletionTime != nil {
								log := ctrl.Log.WithName("PipelineRunController")
								// send PipelineRun completion status to KITE webhook
								if err := r.handlePipelinerunCompletion(context.Background(), newPipelineRun); err != nil {
									log.Error(err, "Failed to send PipelineRun status to KITE", "pipelineRun", newPipelineRun.Name)
								}

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
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}
