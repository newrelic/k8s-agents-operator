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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	healthMonitor *instrumentation.HealthMonitor
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
	logger := log.FromContext(ctx)
	logger.Info("start pod reconciliation", "namespace", req.Namespace, "name", req.Name)

	pod := corev1.Pod{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, &pod)
	logger.Info("pod reconciliation; get", "namespace", req.Namespace, "name", req.Name /*"object", pod,*/, "error", err)
	if apierrors.IsNotFound(err) {
		pod.Name = req.Name
		pod.Namespace = req.Namespace
		logger.Info("pod reconciliation; pod deleted event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.PodRemove(&pod)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod.DeletionTimestamp != nil {
		logger.Info("pod reconciliation; pod deleting event", "namespace", req.Namespace, "name", req.Name)
		r.healthMonitor.PodRemove(&pod)
		return ctrl.Result{}, nil
	}

	logger.Info("pod reconciliation; pod created event", "namespace", req.Namespace, "name", req.Name)
	r.healthMonitor.PodSet(&pod)

	if pod.Status.Phase == corev1.PodPending {
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	//@todo: report this pod ready for some health checks?

	return ctrl.Result{}, nil
}

func (r *PodReconciler) SetupWithHealthMonitor(healthMonitor *instrumentation.HealthMonitor) *PodReconciler {
	r.healthMonitor = healthMonitor
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}).
		For(&corev1.Pod{}).
		Complete(r)
}
