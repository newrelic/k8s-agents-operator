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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/internal/instrumentation"
)

// InstrumentationReconciler reconciles a Instrumentation object
type InstrumentationReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	healthMonitor     *instrumentation.HealthMonitor
	operatorNamespace string
}

//+kubebuilder:rbac:groups=newrelic.com,resources=instrumentations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=newrelic.com,resources=instrumentations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=newrelic.com,resources=instrumentations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Instrumentation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *InstrumentationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	logger.V(2).Info("start instrumentation reconciliation")

	if req.Namespace != r.operatorNamespace {
		return ctrl.Result{}, nil
	}

	inst := v1alpha2.Instrumentation{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, &inst)
	logger.V(2).Info("instrumentation reconciliation; get", "error", err)
	if apierrors.IsNotFound(err) {
		inst.Name = req.Name
		inst.Namespace = req.Namespace
		logger.V(2).Info("instrumentation reconciliation; instrumentation deleted event")
		r.healthMonitor.InstrumentationRemove(&inst)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if inst.DeletionTimestamp != nil {
		logger.V(2).Info("instrumentation reconciliation; instrumentation deleting event")
		r.healthMonitor.InstrumentationRemove(&inst)
		return ctrl.Result{}, nil
	}

	logger.V(2).Info("instrumentation reconciliation; instrumentation created event")
	r.healthMonitor.InstrumentationSet(&inst)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstrumentationReconciler) SetupWithManager(mgr ctrl.Manager, healthMonitor *instrumentation.HealthMonitor, operatorNamespace string) error {
	r.healthMonitor = healthMonitor
	r.operatorNamespace = operatorNamespace
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		For(&v1alpha2.Instrumentation{}).
		WithEventFilter(
			predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
					return r.isInOperatorNamespace(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return r.isInOperatorNamespace(e.ObjectNew)
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return r.isInOperatorNamespace(e.Object)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return r.isInOperatorNamespace(e.Object)
				},
			},
		).
		Complete(r)
}

func (r *InstrumentationReconciler) isInOperatorNamespace(object client.Object) bool {
	inst, ok := object.(*v1alpha2.Instrumentation)
	if !ok {
		return false
	}
	return inst.Namespace == r.operatorNamespace
}
