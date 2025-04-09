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
	mmmetrics "github.com/konflux-ci/mintmaker/internal/pkg/metrics"
)

var (
	MaxSimultaneousPipelineRuns      = 40
	MintMakerGitPlatformLabel        = "mintmaker.appstudio.redhat.com/git-platform"
	MintMakerComponentNameLabel      = "mintmaker.appstudio.redhat.com/component"
	MintMakerComponentNamespaceLabel = "mintmaker.appstudio.redhat.com/namespace"
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

	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")
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
		if pipelineRun.Labels[MintMakerGitPlatformLabel] == label {
			pipelineRuns = append(pipelineRuns, pipelineRun)
		}
	}
	return tektonv1.PipelineRunList{Items: pipelineRuns}
}

func (r *PipelineRunReconciler) updatePipelineRunState(
	pipelineRun tektonv1.PipelineRun, status tektonv1.PipelineRunSpecStatus, errmsg string, ctx context.Context,
) error {

	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")

	originalPipelineRun := pipelineRun.DeepCopy()
	pipelineRun.Spec.Status = status

	// if pipelinerun is to be cancelled, add string with the reason
	if status == tektonv1.PipelineRunSpecStatusCancelled {
		pipelineRun.Status.MarkFailed(string(tektonv1.PipelineRunReasonCancelled), "%s", errmsg)
	}

	patch := client.MergeFrom(originalPipelineRun)
	err := r.Client.Patch(ctx, &pipelineRun, patch)
	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to update pipelinerun status, pipelinerun: %s", pipelineRun.Name))
		return err
	}
	return nil
}

func (r *PipelineRunReconciler) startPipelineRun(plr tektonv1.PipelineRun, ctx context.Context) bool {

	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")

	// if not a github pipelinerun, set pipelinerun in motion and do early return
	if plr.Labels[MintMakerGitPlatformLabel] != "github" {
		log.Info(fmt.Sprintf("PipelineRun will be started (pending state removed): %s", plr.Name))
		_ = r.updatePipelineRunState(plr, "", "", ctx)
		return true
	}

	// proceed with fetching intermediate steps to check token, in case of a github pipelinerun
	var err error
	var component appstudiov1alpha1.Component
	var ghComponent *github.Component

	defer func() {
		if err != nil {
			// log error and cancel pipelinerun
			log.Error(err, err.Error())
			_ = r.updatePipelineRunState(plr, tektonv1.PipelineRunSpecStatusCancelled, err.Error(), ctx)
		}
	}()

	componentName := plr.Labels[MintMakerComponentNameLabel]
	componentNamespace := plr.Labels[MintMakerComponentNamespaceLabel]
	componentKey := types.NamespacedName{Namespace: componentNamespace, Name: componentName}
	err = r.Client.Get(ctx, componentKey, &component)
	if err != nil {
		return false
	}

	ghComponent, err = github.NewComponent(&component, r.Client, ctx)
	if err != nil {
		return false
	}

	// GetToken returns the most current token to be used; it automatically
	// renews and returns the new token, in case the old token got old
	token, err := (*ghComponent).GetToken()
	if err != nil {
		return false
	}
	tokenBString := []byte(token)

	appSecret := corev1.Secret{}
	appSecretKey := types.NamespacedName{Namespace: MintMakerNamespaceName, Name: plr.Name}
	err = r.Client.Get(ctx, appSecretKey, &appSecret)
	if err != nil {
		return false
	}

	// update the token in the secret, in case it was renewed by GetToken
	if !bytes.Equal(appSecret.Data["renovate-token"], tokenBString) {
		originalAppSecret := appSecret.DeepCopy()
		appSecret.Data["renovate-token"] = tokenBString

		secretPatch := client.MergeFrom(originalAppSecret)
		err = r.Client.Patch(ctx, &appSecret, secretPatch)
		if err != nil {
			return false
		}
	}

	// log sucess and set pipelinerun in motion
	log.Info(fmt.Sprintf("PipelineRun will be started (pending state removed): %s", plr.Name))
	_ = r.updatePipelineRunState(plr, "", "", ctx)
	return true
}

func (r *PipelineRunReconciler) launchUpToNPipelineRuns(numToLaunch int, existingPipelineRuns tektonv1.PipelineRunList, ctx context.Context) error {

	if numToLaunch <= 0 {
		return nil
	}
	numLaunched := 0
	for _, pipelineRun := range existingPipelineRuns.Items {
		if pipelineRun.IsPending() {
			success := r.startPipelineRun(pipelineRun, ctx)
			if success {
				numLaunched += 1
				mmmetrics.CountScheduledRuns()
			}
			if numLaunched == numToLaunch {
				break
			}
		}
	}
	return nil
}

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx).WithName("PipelineRunController")
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
