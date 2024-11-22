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
	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha2"
	"github.com/newrelic/k8s-agents-operator/src/instrumentation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *InstrumentationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("start instrumentation reconciliation", "namespace", req.Namespace, "name", req.Name)

	instrumentation := v1alpha2.Instrumentation{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, &instrumentation)
	logger.Info("instrumentation reconciliation; get", "namespace", req.Namespace, "name", req.Name /*"object", instrumentation,*/, "error", err)
	if apierrors.IsNotFound(err) {
		instrumentation.Name = req.Name
		instrumentation.Namespace = req.Namespace
		logger.Info("instrumentation reconciliation; instrumentation deleted event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.InstrumentationRemove(&instrumentation)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if instrumentation.DeletionTimestamp != nil {
		logger.Info("instrumentation reconciliation; instrumentation deleting event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.InstrumentationRemove(&instrumentation)
		return ctrl.Result{}, nil
	}

	logger.Info("instrumentation reconciliation; instrumentation created event", "namespace", req.Namespace, "name", req.Name)
	r.healthMonitor.InstrumentationSet(&instrumentation)

	return ctrl.Result{}, nil
}

func (r *InstrumentationReconciler) SetupWithHealthMonitor(healthMonitor *instrumentation.HealthMonitor) *InstrumentationReconciler {
	r.healthMonitor = healthMonitor
	return r
}

func (r *InstrumentationReconciler) SetupWithOperatorNamespace(operatorNamespace string) *InstrumentationReconciler {
	r.operatorNamespace = operatorNamespace
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstrumentationReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		// @todo: log this, it should never happen
	}
	return inst.Namespace == r.operatorNamespace
}
