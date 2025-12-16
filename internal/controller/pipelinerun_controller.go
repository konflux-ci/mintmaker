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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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
}

// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// We only react to Update events for PipelineRun in mintmaker namespace.
	// Namespace filtering is handled by the manager's cache configuration.
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1.PipelineRun{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
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
