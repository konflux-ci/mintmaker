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

	"github.com/davecgh/go-spew/spew"
	. "github.com/konflux-ci/mintmaker/pkg/common"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	log.Info("hello from PipelineRunReconciler.Reconcile") //FIXME debug, remove after done

	if req.Namespace != MintMakerNamespaceName {
		return ctrl.Result{}, nil
	}

	//FIXME get number of running pipelines
	pipelinerun := &tektonv1beta1.PipelineRun{}
	err := r.Client.Get(ctx, req.NamespacedName, pipelinerun)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}
	log.Info(fmt.Sprintf("pipelinerun: %v", pipelinerun)) //FIXME debug, remove after done
	log.Info(spew.Sdump(pipelinerun))                     //FIXME debug, remove after done
	//TODO okay, let's try to run this controller, and see what is stored in 'pipelinerun'... is there info about the current pipelines?

	// running_pprns := 19

	//FIXME compare to number of allowed pipelines
	// if running_pprns < MaxSimultaneousPipelineRuns {
	// 	//FIXME delete 'pending' status of one pipeline

	// }

	//FIXME check if this log message prints something useful
	log.Info(fmt.Sprintf("PipelineRun is updated: %v", req.NamespacedName))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {

	fmt.Println("hello from PipelineRunReconciler.SetupWithManager") //FIXME debug, remove after done

	return ctrl.NewControllerManagedBy(mgr).
		For(&tektonv1beta1.PipelineRun{}).
		/* TODO: For the time being we just ignore all types of events
		In a second implementation we are going to listen to changes in the pipelinerun
		and one of these events will have to return true, and the reconcile can be stuff
		based on the event */
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(createEvent event.CreateEvent) bool { return false },
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return false },
			UpdateFunc:  func(updateEvent event.UpdateEvent) bool { return true },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
