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
	"github.com/newrelic/k8s-agents-operator/src/instrumentation"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NamespaceReconciler reconciles a Pod object
type NamespaceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	healthMonitor *instrumentation.HealthMonitor
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Instrumentation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("start namespace reconciliation", "namespace", req.Namespace, "name", req.Name)

	ns := corev1.Namespace{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, &ns)
	logger.Info("namespace reconciliation; get", "namespace", req.Namespace, "name", req.Name /*"object", ns,*/, "error", err)
	if apierrors.IsNotFound(err) {
		ns.Name = req.Name
		logger.Info("namespace reconciliation; namespace deleted event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.NamespaceRemove(&ns)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if ns.DeletionTimestamp != nil {
		logger.Info("namespace reconciliation; namespace deleting event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.NamespaceSet(&ns)
		return ctrl.Result{}, nil
	}

	logger.Info("namespace reconciliation; namespace created event", "namespace", req.Namespace, "name", req.Name)
	r.healthMonitor.NamespaceSet(&ns)

	//@todo: track namespaces

	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) SetupWithHealthMonitor(healthMonitor *instrumentation.HealthMonitor) *NamespaceReconciler {
	r.healthMonitor = healthMonitor
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		For(&corev1.Namespace{}).
		WithEventFilter(
			predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
					return r.isNamespace(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return r.isNamespace(e.ObjectNew)
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return r.isNamespace(e.Object)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return r.isNamespace(e.Object)
				},
			},
		).
		Complete(r)
}

func (r *NamespaceReconciler) isNamespace(object client.Object) bool {
	_, ok := object.(*corev1.Namespace)
	return ok
}