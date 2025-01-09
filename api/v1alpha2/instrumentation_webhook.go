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
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

// log is for logging in this package.
var instrumentationLog logr.Logger

// SetupWebhookWithManager will set up the manager to manage the webhooks
func (r *Instrumentation) SetupWebhookWithManager(mgr ctrl.Manager, logger logr.Logger) error {
	instrumentationLog = logger
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(r).
		WithDefaulter(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-newrelic-com-v1alpha2-instrumentation,mutating=true,failurePolicy=fail,sideEffects=None,groups=newrelic.com,resources=instrumentations,verbs=create;update,versions=v1alpha2,name=minstrumentation.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &Instrumentation{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Instrumentation) Default(ctx context.Context, obj runtime.Object) error {
	inst, ok := obj.(*Instrumentation)
	if !ok {
		return fmt.Errorf("expected an Instrumentation object but got %T", obj)
	}

	instrumentationLog.Info("Defaulting for Instrumentation", "name", inst.GetName())
	if inst.Labels == nil {
		inst.Labels = map[string]string{}
	}
	if inst.Labels["app.kubernetes.io/managed-by"] == "" {
		inst.Labels["app.kubernetes.io/managed-by"] = "k8s-agents-operator"
	}
	if inst.Spec.LicenseKeySecret == "" {
		inst.Spec.LicenseKeySecret = "newrelic-key-secret"
	}
	return nil
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
//+kubebuilder:webhook:verbs=create;update,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=fail,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationcreateupdate.kb.io,sideEffects=none,admissionReviewVersions=v1
//+kubebuilder:webhook:verbs=delete,path=/validate-newrelic-com-v1alpha2-instrumentation,mutating=false,failurePolicy=ignore,groups=newrelic.com,resources=instrumentations,versions=v1alpha2,name=vinstrumentationdelete.kb.io,sideEffects=none,admissionReviewVersions=v1

const (
	envNewRelicPrefix = "NEW_RELIC_"
	envOtelPrefix     = "OTEL_"
)

var validEnvPrefixes = []string{envNewRelicPrefix, envOtelPrefix}
var validEnvPrefixesStr = strings.Join(validEnvPrefixes, ", ")

var _ webhook.CustomValidator = &Instrumentation{}

// ValidateCreate to validate the creation operation
func (r *Instrumentation) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*Instrumentation)
	instrumentationLog.Info("validate create", "name", inst.Name)
	return r.validate(inst)
}

// ValidateUpdate to validate the update operation
func (r *Instrumentation) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	inst := newObj.(*Instrumentation)
	instrumentationLog.Info("validate update", "name", inst.Name)
	return r.validate(inst)
}

// ValidateDelete to validate the deletion operation
func (r *Instrumentation) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst := obj.(*Instrumentation)
	instrumentationLog.Info("validate delete", "name", inst.Name)
	return r.validate(inst)
}

// validate to validate all the fields
func (r *Instrumentation) validate(inst *Instrumentation) (admission.Warnings, error) {
	// TODO: Maybe improve this
	acceptableLangs := []string{"dotnet", "go", "java", "nodejs", "php-7.2", "php-7.3", "php-7.4", "php-8.0", "php-8.1", "php-8.2", "php-8.3", "python", "ruby"}
	agentLang := inst.Spec.Agent.Language
	if !slices.Contains(acceptableLangs, agentLang) {
		return nil, fmt.Errorf("instrumentation agent language %q must be one of the accepted languages (%s)", agentLang, strings.Join(acceptableLangs, ", "))
	}

	if err := r.validateEnv(inst.Spec.Agent.Env); err != nil {
		return nil, err
	}

	if inst.Spec.Agent.IsEmpty() {
		return nil, fmt.Errorf("instrumentation %q agent is empty", inst.Name)
	}

	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.PodLabelSelector); err != nil {
		return nil, err
	}
	if _, err := metav1.LabelSelectorAsSelector(&inst.Spec.NamespaceLabelSelector); err != nil {
		return nil, err
	}

	return nil, nil
}

// validateEnv to validate the environment variables used all start with the required prefixes
func (r *Instrumentation) validateEnv(envs []corev1.EnvVar) error {
	var invalidNames []string
	for _, env := range envs {
		var valid bool
		for _, validEnvPrefix := range validEnvPrefixes {
			if strings.HasPrefix(env.Name, validEnvPrefix) {
				valid = true
				break
			}
		}
		if !valid {
			invalidNames = append(invalidNames, env.Name)
		}
	}
	if len(invalidNames) > 0 {
		return fmt.Errorf("env name should start with %s; found these invalid names %s", validEnvPrefixesStr, strings.Join(invalidNames, ", "))
	}
	return nil
}
