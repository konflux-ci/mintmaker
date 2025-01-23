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
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var (
	MaxSimultaneousPipelineRuns = 20
	MintMakerAppstudioLabel     = "mintmaker.appstudio.redhat.com/platform"
)

// Collect pipelineruns with state 'running' or 'started'
func countRunningPipelineRuns(existingPipelineRuns tektonv1.PipelineRunList) (int, error) {
	var runningPipelineRuns []*tektonv1.PipelineRun
	for i, pipelineRun := range existingPipelineRuns.Items {
		if len(pipelineRun.Status.Conditions) > 0 &&
			pipelineRun.Status.Conditions[0].Status == corev1.ConditionUnknown &&
			(pipelineRun.Status.Conditions[0].Reason == tektonv1.PipelineRunReasonRunning.String() ||
				pipelineRun.Status.Conditions[0].Reason == tektonv1.PipelineRunReasonStarted.String()) {
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

func (r *PipelineRunReconciler) listExistingPipelineRuns(ctx context.Context, req ctrl.Request) (tektonv1.PipelineRunList, error) {

	log := ctrllog.FromContext(ctx).WithName("PipelineRun")
	var existingPipelineRuns tektonv1.PipelineRunList

	err := r.Client.List(ctx, &existingPipelineRuns, client.InNamespace(req.Namespace))
	if err != nil {
		log.Error(err, "Unable to list existing pipelineruns")
		return tektonv1.PipelineRunList{}, err
	}
	return existingPipelineRuns, nil
}

func filterPipelineRunListWithLabel(pipelineRunList tektonv1.PipelineRunList, label string) tektonv1.PipelineRunList {
	pipelineRuns := []tektonv1.PipelineRun{}

	for _, pipelineRun := range pipelineRunList.Items {
		if pipelineRun.Labels[MintMakerAppstudioLabel] == label {
			pipelineRuns = append(pipelineRuns, pipelineRun)
		}
	}
	return tektonv1.PipelineRunList{Items: pipelineRuns}
}

func (r *PipelineRunReconciler) launchUpToNPipelineRuns(numToLaunch int, existingPipelineRuns tektonv1.PipelineRunList, ctx context.Context) error {

	log := ctrllog.FromContext(ctx).WithName("PipelineRun")
	ctx = ctrllog.IntoContext(ctx, log)

	if numToLaunch <= 0 {
		return nil
	}
	numLaunched := 0
	for _, pipelineRun := range existingPipelineRuns.Items {
		if pipelineRun.IsPending() {
			original := pipelineRun.DeepCopy()
			pipelineRun.Spec.Status = ""
			patch := client.MergeFrom(original)
			err := r.Client.Patch(ctx, &pipelineRun, patch)
			if err != nil {
				log.Error(err, "Unable to update pipelinerun status")
				return err
			}
			log.Info(fmt.Sprintf("PipelineRun is updated (pending state removed): %s", pipelineRun.Name))
			numLaunched += 1
			if numLaunched == numToLaunch {
				break
			}
		}
	}

	return nil
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("PipelineRun")
	ctx = ctrllog.IntoContext(ctx, log)

	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	// Get pipelineruns
	existingPipelineRuns, err := r.listExistingPipelineRuns(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	numRunning, err := countRunningPipelineRuns(existingPipelineRuns)
	if err != nil {
		log.Error(err, "Unable to count running pipelineruns")
		return ctrl.Result{}, err
	}

	githubPipelineRuns := filterPipelineRunListWithLabel(existingPipelineRuns, "github")

	// Launch up to N pipelineruns, 'github' ones first
	numToLaunch := MaxSimultaneousPipelineRuns - numRunning
	err = r.launchUpToNPipelineRuns(numToLaunch, githubPipelineRuns, ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	numRunning, err = countRunningPipelineRuns(existingPipelineRuns)
	if err != nil {
		log.Error(err, "Unable to count running pipelineruns")
		return ctrl.Result{}, err
	}
	numToLaunch = MaxSimultaneousPipelineRuns - numRunning
	err = r.launchUpToNPipelineRuns(numToLaunch, existingPipelineRuns, ctx)
	if err != nil {
		return ctrl.Result{}, err
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
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool { return false },
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				if oldPipelineRun, ok := updateEvent.ObjectOld.(*tektonv1.PipelineRun); ok {
					if newPipelineRun, ok := updateEvent.ObjectNew.(*tektonv1.PipelineRun); ok {
						return oldPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown() && !newPipelineRun.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
					}
				}
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
