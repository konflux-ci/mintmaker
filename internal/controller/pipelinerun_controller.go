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
	"time"

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
	MaxSimultaneousPipelineRuns = 2 //FIXME revert to 20
)

// Collect pipelineruns with state 'running' or 'started'
func countRunningPipelineRuns(existingPipelineRuns tektonv1beta1.PipelineRunList) (int, error) {
	var runningPipelineRuns []*tektonv1beta1.PipelineRun
	for i, pipelineRun := range existingPipelineRuns.Items {
		if len(pipelineRun.Status.Conditions) > 0 &&
			pipelineRun.Status.Conditions[0].Status == corev1.ConditionUnknown &&
			(pipelineRun.Status.Conditions[0].Reason == tektonv1beta1.PipelineRunReasonRunning.String() ||
				pipelineRun.Status.Conditions[0].Reason == tektonv1beta1.PipelineRunReasonStarted.String()) {
			runningPipelineRuns = append(runningPipelineRuns, &existingPipelineRuns.Items[i])
		}
	}
	numRunning := len(runningPipelineRuns)

	return numRunning, nil
}

// PipelineRunReconciler reconciles a PipelineRun object
type PipelineRunReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("PipelineRun")
	ctx = ctrllog.IntoContext(ctx, log)

	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	// Get pipelineruns
	var existingPipelineRuns tektonv1beta1.PipelineRunList
	err := r.Client.List(ctx, &existingPipelineRuns, client.InNamespace(req.Namespace))
	if err != nil {
		log.Error(err, "Unable to list child pipelineruns")
		return ctrl.Result{}, err
	}

	numRunning, err := countRunningPipelineRuns(existingPipelineRuns)
	if err != nil {
		log.Error(err, "Unable to count running pipelineruns")
		return ctrl.Result{}, err
	}

	// Launch up to N pipelineruns
	numToLaunch := MaxSimultaneousPipelineRuns - numRunning
	if numToLaunch > 0 {
		numLaunched := 0
		for _, pipelineRun := range existingPipelineRuns.Items {
			if pipelineRun.IsPending() {
				pipelineRun.Spec.Status = ""
				err = r.Client.Update(ctx, &pipelineRun, &client.UpdateOptions{})
				if err != nil {
					log.Error(err, "Unable to update pipelinerun status")
					return ctrl.Result{}, err
				}
				log.Info(fmt.Sprintf("PipelineRun is updated (pending state removed): %v", req.NamespacedName))
				numLaunched += 1
				if numLaunched == numToLaunch {
					break
				}
			}
		}
	}
	// without this pause, the controller might launch more pipelines than the max number,
	// probably because it takes a moment until current states are synchronized
	time.Sleep(100 * time.Millisecond)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1beta1.PipelineRun{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(createEvent event.CreateEvent) bool { return true },
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return false },
			UpdateFunc:  func(updateEvent event.UpdateEvent) bool { return true },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
