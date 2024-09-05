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
	"errors"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/src/api/v1alpha1"
	"github.com/newrelic/k8s-agents-operator/src/internal/webhookhandler"
)

var (
	errMultipleInstancesPossible = errors.New("multiple New Relic Instrumentation instances available, cannot determine which one to select")
	errNoInstancesAvailable      = errors.New("no New Relic Instrumentation instances available")
)

type instPodMutator struct {
	Client      client.Client
	sdkInjector *sdkInjector
	Logger      logr.Logger
}

var _ webhookhandler.PodMutator = (*instPodMutator)(nil)

func NewMutator(logger logr.Logger, client client.Client) *instPodMutator {
	return &instPodMutator{
		Logger: logger,
		Client: client,
		sdkInjector: &sdkInjector{
			logger: logger,
			client: client,
		},
	}
}

func (pm *instPodMutator) Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	logger := pm.Logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var inst *v1alpha1.Instrumentation
	var err error

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}

	// TODO We the user should list the containers if more than one is running
	pod = pm.sdkInjector.inject(ctx, inst, ns, pod, "")
	// Assure Secret Existence
	err = pm.replicateSecret(ctx, ns, pod)
	if err != nil {
		logger.Error(err, "failed to replicate secret")
	}

	return pod, nil
}

func (pm *instPodMutator) replicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) error {
	logger := pm.Logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var secret corev1.Secret

	err := pm.Client.Get(ctx, client.ObjectKey{Name: pod.Spec.ServiceAccountName}, &secret)
	if err != nil {
		logger.Error(err, "failed to retrieve the secret")
	}

	err = pm.Client.Create(ctx, &secret)
	if err != nil {
		logger.Error(err, "failed to create a new secret")
	}

	return nil
}

func (pm *instPodMutator) getInstrumentationInstance(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (*v1alpha1.Instrumentation, error) {
	logger := pm.Logger.WithValues("namespace", pod.Namespace, "name", pod.Name)

	var listInst v1alpha1.InstrumentationList
	if err := pm.Client.List(ctx, &listInst, client.InNamespace(ns.Name)); err != nil {
		return nil, err
	}

	for _, inst := range listInst.Items {
		selector, err := metav1.LabelSelectorAsSelector(&inst.Spec.PodLabelSelector)
		if err != nil {
			logger.Error(err, "failed to parse label selector %s: %s", inst.Name, err)
			continue
		}
		// TODO we should decide what to do if multiple rule matches
		if selector.Matches(fields.Set(pod.Labels)) {
			return &inst, nil
		}
	}
	return nil, nil
}
