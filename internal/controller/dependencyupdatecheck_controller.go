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

	mmv1alpha1 "github.com/konflux-ci/mintmaker/api/v1alpha1"
	. "github.com/konflux-ci/mintmaker/pkg/common"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	RenovateImageEnvName    = "RENOVATE_IMAGE"
	DefaultRenovateImageUrl = "quay.io/konflux-ci/mintmaker-renovate-image:latest"
)

// DependencyUpdateCheckReconciler reconciles a DependencyUpdateCheck object
type DependencyUpdateCheckReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func NewDependencyUpdateCheckReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder) *DependencyUpdateCheckReconciler {
	return &DependencyUpdateCheckReconciler{
		Client: client,
		Scheme: scheme,
	}
}

// createPipelineRun creates and returns a new PipelineRun
func (r *DependencyUpdateCheckReconciler) createPipelineRun(ctx context.Context) (*tektonv1beta1.PipelineRun, error) {

	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("renovate-pipelinerun-%d-%s", timestamp, RandomString(5))

	// Creating the pipelineRun definition
	pipelineRun := &tektonv1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: MintMakerNamespaceName,
		},
		Spec: tektonv1beta1.PipelineRunSpec{
			Status: tektonv1beta1.PipelineRunSpecStatusPending,
			PipelineSpec: &tektonv1beta1.PipelineSpec{
				Tasks: []tektonv1beta1.PipelineTask{
					{
						Name: "build",
						TaskSpec: &tektonv1beta1.EmbeddedTask{
							TaskSpec: tektonv1beta1.TaskSpec{
								Steps: []tektonv1beta1.Step{
									{
										Name:  "renovate",
										Image: DefaultRenovateImageUrl,
										Script: `
	                                    echo "Running Renovate"
	                                    sleep 10
	                                `,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := r.Client.Create(ctx, pipelineRun); err != nil {
		return nil, err
	}

	log.Info(fmt.Sprintf("Created pipelinerun %s", name))
	return pipelineRun, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *DependencyUpdateCheckReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := ctrllog.FromContext(ctx).WithName("DependencyUpdateCheckController")
	ctx = ctrllog.IntoContext(ctx, log)

	// Ignore CRs that are not from the mintmaker namespace
	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	dependencyupdatecheck := &mmv1alpha1.DependencyUpdateCheck{}
	err := r.Client.Get(ctx, req.NamespacedName, dependencyupdatecheck)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	// If the DependencyUpdateCheck has been handled before, skip it
	if value, exists := dependencyupdatecheck.Annotations[MintMakerProcessedAnnotationName]; exists && value == "true" {
		log.Info(fmt.Sprintf("DependencyUpdateCheck has been processed: %v", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	// Update the DependencyUpdateCheck to add a processed annotation
	log.Info(fmt.Sprintf("new DependencyUpdateCheck found: %v", req.NamespacedName))
	if dependencyupdatecheck.Annotations == nil {
		dependencyupdatecheck.Annotations = map[string]string{}
	}
	dependencyupdatecheck.Annotations[MintMakerProcessedAnnotationName] = "true"

	err = r.Client.Update(ctx, dependencyupdatecheck)
	if err != nil {
		log.Error(err, "failed to update DependencyUpdateCheck annotations")
		return ctrl.Result{}, nil
	}

	log.Info("Creating pending pipeline runs")
	pipelinerun, err := r.createPipelineRun(ctx)
	if err != nil {
		log.Error(err, "failed to create pipelineruns")
	} else {
		log.Info(fmt.Sprintf("Created pipelinerun %v", pipelinerun))
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DependencyUpdateCheckReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// we are monitoring the creation of DependencyUpdateCheck
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1alpha1.DependencyUpdateCheck{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(createEvent event.CreateEvent) bool { return true },
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return false },
			UpdateFunc:  func(updateEvent event.UpdateEvent) bool { return false },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
