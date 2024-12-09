/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	. "github.com/konflux-ci/mintmaker/pkg/common"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	MaxSimultaneousPipelineRuns = 20
)

// PipelineRunReconciler reconciles a PipelineRun object
type PipelineRunReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PipelineRun object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("PipelineRun")
	ctx = ctrllog.IntoContext(ctx, log)

	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	// Get pipelineruns
	var childPipelineRuns tektonv1beta1.PipelineRunList
	err := r.Client.List(ctx, &childPipelineRuns, client.InNamespace(req.Namespace))
	if err != nil {
		log.Error(err, "Unable to list child pipelineruns")
		return ctrl.Result{}, err
	}

	// Collect pipelineruns with state 'running' or 'started'
	var runningPipelineRuns []*tektonv1beta1.PipelineRun
	for i, pipelineRun := range childPipelineRuns.Items {
		if len(pipelineRun.Status.Conditions) > 0 &&
			pipelineRun.Status.Conditions[0].Status == corev1.ConditionUnknown &&
			(pipelineRun.Status.Conditions[0].Reason == tektonv1beta1.PipelineRunReasonRunning.String() ||
				pipelineRun.Status.Conditions[0].Reason == tektonv1beta1.PipelineRunReasonStarted.String()) {
			runningPipelineRuns = append(runningPipelineRuns, &childPipelineRuns.Items[i])
		}
	}
	numRunning := len(runningPipelineRuns)

	// Launch one pipelinerun if less than the maximum number is currently running
	if numRunning < MaxSimultaneousPipelineRuns {
		for _, pipelineRun := range childPipelineRuns.Items {
			if pipelineRun.IsPending() {
				pipelineRun.Spec.Status = ""
				err = r.Client.Update(ctx, &pipelineRun, &client.UpdateOptions{})
				if err != nil {
					log.Error(err, "Unable to update pipelinerun status")
					return ctrl.Result{}, err
				}
				log.Info(fmt.Sprintf("\n\nPipelineRun is updated (pending state removed): %v", req.NamespacedName))
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1beta1.PipelineRun{}).
		/* TODO: For the time being we just ignore all types of events
		In a second implementation we are going to listen to changes in the pipelinerun
		and one of these events will have to return true, and the reconcile can be stuff
		based on the event */
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(createEvent event.CreateEvent) bool { return true },
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return true },
			UpdateFunc:  func(updateEvent event.UpdateEvent) bool { return true },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
