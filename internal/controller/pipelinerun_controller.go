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
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

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
	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")
	ctx = ctrllog.IntoContext(ctx, log)

	// Get the PipelineRun object
	var pipelineRun tektonv1.PipelineRun
	if err := r.Client.Get(ctx, req.NamespacedName, &pipelineRun); err != nil {
		// PipelineRun was deleted, nothing to do
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only process completed PipelineRuns
	if !pipelineRun.IsDone() {
		return ctrl.Result{}, nil
	}

	// Check if the PipelineRun failed
	condition := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
	if condition != nil && !condition.IsTrue() {
		// send failure webhook to KITE
		if r.KiteClient != nil {
			if err := r.sendFailureWebhook(ctx, &pipelineRun); err != nil {
				fmt.Printf("\nfailed to send pipeline failure webhook for pipelineRun %s: %s\n", pipelineRun.Name, err)
			}
		} else {
			fmt.Printf("\nKITE client not available, skipping webhook notification for pipelineRun %s\n", pipelineRun.Name)
		}
	} else if condition != nil && condition.IsTrue() {
		// send success webhook to KITE
		log.Info("PipelineRun succeeded", "pipelineRun", pipelineRun.Name)

		if r.KiteClient != nil {
			if err := r.sendSuccessWebhook(ctx, &pipelineRun); err != nil {
				fmt.Printf("\nfailed to send pipeline success webhook for pipelineRun %s: %s\n", pipelineRun.Name, err)
			}
		} else {
			fmt.Printf("\nKITE client not available, skipping webhook notification for pipelineRun %s\n", pipelineRun.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *PipelineRunReconciler) sendFailureWebhook(ctx context.Context, pipelineRun *tektonv1.PipelineRun) error {
	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")
	ctx = ctrllog.IntoContext(ctx, log)

	// Get detailed failure reason from TaskRun statuses
	reason := pipelineRun.Status.GetCondition(apis.ConditionSucceeded).GetReason()
	log.Info("PipelineRun failed", "pipelineRun", pipelineRun.Name, "reason", reason)

	payload := kite.PipelineFailurePayload{
		PipelineName:  pipelineRun.Name,
		Namespace:     pipelineRun.Namespace,
		FailureReason: reason,
		RunID:         pipelineRun.Labels[MintMakerComponentNameLabel] + "-" + pipelineRun.Name,
		LogsURL:       "", // Placeholder for logs URL if available
	}

	return r.KiteClient.SendPipelineFailure(ctx, payload)
}

func (r *PipelineRunReconciler) sendSuccessWebhook(ctx context.Context, pipelineRun *tektonv1.PipelineRun) error {
	payload := kite.PipelineSuccessPayload{
		PipelineName: pipelineRun.Name,
		Namespace:    pipelineRun.Namespace,
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
