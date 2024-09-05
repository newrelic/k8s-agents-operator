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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	AnnotationDefaultAutoInstrumentationJava   = "instrumentation.newrelic.com/default-auto-instrumentation-java-image"
	AnnotationDefaultAutoInstrumentationNodeJS = "instrumentation.newrelic.com/default-auto-instrumentation-nodejs-image"
	AnnotationDefaultAutoInstrumentationPython = "instrumentation.newrelic.com/default-auto-instrumentation-python-image"
	AnnotationDefaultAutoInstrumentationDotNet = "instrumentation.newrelic.com/default-auto-instrumentation-dotnet-image"
	AnnotationDefaultAutoInstrumentationPhp    = "instrumentation.newrelic.com/default-auto-instrumentation-php-image"
	AnnotationDefaultAutoInstrumentationRuby   = "instrumentation.newrelic.com/default-auto-instrumentation-ruby-image"
	AnnotationDefaultAutoInstrumentationGo     = "instrumentation.newrelic.com/default-auto-instrumentation-go-image"
	envNewRelicPrefix                          = "NEW_RELIC_"
	envOtelPrefix                              = "OTEL_"
)

// log is for logging in this package.
var instrumentationlog = logf.Log.WithName("instrumentation-resource")

func (r *Instrumentation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-newrelic-com-v1alpha1-instrumentation,mutating=true,failurePolicy=fail,sideEffects=None,groups=newrelic.com,resources=instrumentations,verbs=create;update,versions=v1alpha1,name=instrumentation.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Instrumentation{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *Instrumentation) Default() {
	instrumentationlog.Info("default", "name", r.Name)
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	if r.Labels["app.kubernetes.io/managed-by"] == "" {
		r.Labels["app.kubernetes.io/managed-by"] = "k8s-agents-operator"
	}
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-newrelic-com-v1alpha1-instrumentation,mutating=false,failurePolicy=fail,groups=newrelic.com,resources=instrumentations,versions=v1alpha1,name=vinstrumentationcreateupdate.kb.io,sideEffects=none,admissionReviewVersions=v1
// +kubebuilder:webhook:verbs=delete,path=/validate-newrelic-com-v1alpha1-instrumentation,mutating=false,failurePolicy=ignore,groups=newrelic.com,resources=instrumentations,versions=v1alpha1,name=vinstrumentationdelete.kb.io,sideEffects=none,admissionReviewVersions=v1

var _ webhook.Validator = &Instrumentation{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateCreate() error {
	instrumentationlog.Info("validate create", "name", r.Name)
	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateUpdate(old runtime.Object) error {
	instrumentationlog.Info("validate update", "name", r.Name)
	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *Instrumentation) ValidateDelete() error {
	instrumentationlog.Info("validate delete", "name", r.Name)
	return nil
}

func (r *Instrumentation) validate() error {
	return nil
}
