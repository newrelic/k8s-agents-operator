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

package v1alpha2

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

const (
	envNewRelicPrefix = "NEW_RELIC_"
	envOtelPrefix     = "OTEL_"
)

// log is for logging in this package.
var instrumentationlog = logf.Log.WithName("instrumentation-resource")

func (r *Instrumentation) SetupWebhookWithManager(mgr ctrl.Manager, defaulter webhook.CustomDefaulter, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(validator).
		WithDefaulter(defaulter).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-newrelic-com-v1alpha2-instrumentation,mutating=true,failurePolicy=fail,sideEffects=None,groups=newrelic.com,resources=instrumentations,verbs=create;update,versions=v1alpha2,name=minstrumentation.kb.io,admissionReviewVersions=v1
var _ webhook.Defaulter = (*Instrumentation)(nil)

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *Instrumentation) Default() {
	instrumentationlog.V(1).Info("default", "name", r.Name)
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	if r.Labels["app.kubernetes.io/managed-by"] == "" {
		r.Labels["app.kubernetes.io/managed-by"] = "k8s-agents-operator"
	}

	if r.Spec.LicenseKeySecret == "" {
		r.Spec.LicenseKeySecret = "newrelic-key-secret"
	}
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=fail,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationcreateupdate.kb.io,sideEffects=none,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=delete,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=ignore,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationdelete.kb.io,sideEffects=none,admissionReviewVersions=v1

var _ webhook.Validator = (*Instrumentation)(nil)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateCreate() (admission.Warnings, error) {
	instrumentationlog.V(1).Info("validate_create", "name", r.Name)
	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateUpdate(oldObj runtime.Object) (admission.Warnings, error) {
	instrumentationlog.V(1).Info("validate_update", "name", r.Name)
	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateDelete() (admission.Warnings, error) {
	instrumentationlog.V(1).Info("validate_delete", "name", r.Name)
	return nil, nil
}

func (r *Instrumentation) validate() (admission.Warnings, error) {
	if err := r.validateEnv(r.Spec.Agent.Env); err != nil {
		return nil, err
	}

	if r.Spec.Agent.IsEmpty() {
		return nil, fmt.Errorf("instrumentation %q agent is empty", r.Name)
	}

	if _, err := metav1.LabelSelectorAsSelector(&r.Spec.PodLabelSelector); err != nil {
		return nil, err
	}
	if _, err := metav1.LabelSelectorAsSelector(&r.Spec.NamespaceLabelSelector); err != nil {
		return nil, err
	}

	return nil, nil
}

func (r *Instrumentation) validateEnv(envs []corev1.EnvVar) error {
	for _, env := range envs {
		if !strings.HasPrefix(env.Name, envNewRelicPrefix) && !strings.HasPrefix(env.Name, envOtelPrefix) {
			return fmt.Errorf("env name should start with \"NEW_RELIC_\" or \"OTEL_\": %s", env.Name)
		}
	}
	return nil
}
