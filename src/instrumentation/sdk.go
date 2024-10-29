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

package instrumentation

import (
	"context"
	"fmt"

	"runtime/debug"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/apm"
)

const (
	DefaultLicenseKeySecretName = "newrelic-key-secret"
)

// compile time type assertion
var _ SdkInjector = (*NewrelicSdkInjector)(nil)

// SdkInjector is used to inject our instrumentation into a pod
type SdkInjector interface {
	Inject(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod
}

// NewrelicSdkInjector is the base struct used to inject our instrumentation into a pod
type NewrelicSdkInjector struct {
	client           client.Client
	logger           logr.Logger
	injectorRegistry *apm.InjectorRegistery
}

// NewNewrelicSdkInjector is used to create our injector
func NewNewrelicSdkInjector(logger logr.Logger, client client.Client, injectorRegistry *apm.InjectorRegistery) *NewrelicSdkInjector {
	return &NewrelicSdkInjector{
		client:           client,
		logger:           logger,
		injectorRegistry: injectorRegistry,
	}
}

// Inject is used to utilize a list of instrumentations, and if the injectors language matches the instrumentation, trigger the injector
func (i *NewrelicSdkInjector) Inject(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	hadMatchingInjector := false
	for _, inst := range insts {
		for _, injector := range i.injectorRegistry.GetInjectors() {
			mutatedPod, matchedThisInjector, err := i.injectWithInjector(ctx, injector, inst, ns, pod)
			hadMatchingInjector = hadMatchingInjector || matchedThisInjector
			if err != nil {
				i.logger.Error(err, "Skipping agent injection", "agent_language", inst.Spec.Agent.Language)
				continue
			}
			pod = mutatedPod
		}
	}
	if !hadMatchingInjector {
		i.logger.Info("No language agents found while trying to instrument pod",
			"pod_details", pod.String(),
			"pod_namespace", pod.Namespace,
			"registered_injectors", i.injectorRegistry.GetInjectors().Names(),
		)
	}
	return pod
}

func (i *NewrelicSdkInjector) injectWithInjector(ctx context.Context, injector apm.Injector, inst *v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (mutatedPod corev1.Pod, hadMatchingInjector bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v, stacktrace: %s", r, debug.Stack())
		}
	}()

	if injector.Language() != inst.Spec.Agent.Language {
		return pod, false, nil
	}
	injector.ConfigureClient(i.client)
	injector.ConfigureLogger(i.logger.WithValues("injector", injector.Language()))
	i.logger.V(1).Info("injecting instrumentation into pod",
		"agent_language", inst.Spec.Agent.Language,
		"newrelic-namespace", inst.Namespace,
		"newrelic-name", inst.Name,
	)

	mutatedPod, err = injector.Inject(ctx, *inst, ns, pod)
	return mutatedPod, true, err
}
