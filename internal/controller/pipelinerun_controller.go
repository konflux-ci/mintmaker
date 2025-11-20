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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	"github.com/konflux-ci/mintmaker/internal/pkg/config"
	. "github.com/konflux-ci/mintmaker/internal/pkg/constant"
)

var (
	MintMakerGitPlatformLabel        = "mintmaker.appstudio.redhat.com/git-platform"
	MintMakerComponentNameLabel      = "mintmaker.appstudio.redhat.com/component"
	MintMakerComponentNamespaceLabel = "mintmaker.appstudio.redhat.com/namespace"
)

// PipelineRunReconciler reconciles a PipelineRun object
type PipelineRunReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Config *config.ControllerConfig
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
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
									"PipelineRun finished", "pipelineRun", newPipelineRun.Name,
									"component", newPipelineRun.Labels[MintMakerComponentNameLabel],
									"componentNamespace", newPipelineRun.Labels[MintMakerComponentNamespaceLabel],
									"repository", strings.ReplaceAll(newPipelineRun.Labels["mintmaker.appstudio.redhat.com/repository"], "_", "/"),
									"gitHost", newPipelineRun.Labels["mintmaker.appstudio.redhat.com/git-host"],
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
