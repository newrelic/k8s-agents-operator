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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/newrelic/k8s-agents-operator/internal/instrumentation"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	healthMonitor     *instrumentation.HealthMonitor
	operatorNamespace string
}

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Instrumentation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name).V(2)

	if slices.Contains([]string{"kube-system", "newrelic"}, req.Namespace) {
		return ctrl.Result{}, nil
	}

	logger.Info("start pod reconciliation")

	pod := corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, &pod)
	logger.Info("pod reconciliation; get", "error", err)
	if apierrors.IsNotFound(err) {
		pod.Name = req.Name
		pod.Namespace = req.Namespace
		logger.Info("pod reconciliation; pod deleted event")
		r.healthMonitor.PodRemove(&pod)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod.DeletionTimestamp != nil {
		logger.Info("pod reconciliation; pod deleting event")
		r.healthMonitor.PodRemove(&pod)
		return ctrl.Result{}, nil
	}

	logger.Info("pod reconciliation; pod created event", "namespace", req.Namespace, "name", req.Name)
	r.healthMonitor.PodSet(&pod)

	if pod.Status.Phase == corev1.PodPending {
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager, healthMonitor *instrumentation.HealthMonitor, operatorNamespace string) error {
	r.healthMonitor = healthMonitor
	r.operatorNamespace = operatorNamespace
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		For(&corev1.Pod{}).
		WithEventFilter(
			predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool {
					return r.isPod(e.Object)
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return r.isPod(e.ObjectNew)
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return r.isPod(e.Object)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return r.isPod(e.Object)
				},
			},
		).
		Complete(r)
}

func (r *PodReconciler) isPod(object client.Object) bool {
	ns, ok := object.(*corev1.Pod)
	if !ok {
		return false
	}
	if slices.Contains([]string{"kube-system", r.operatorNamespace}, ns.Name) {
		return false
	}
	return true
}
