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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic-experimental/newrelic-agent-operator/api/v1alpha1"
	"github.com/newrelic-experimental/newrelic-agent-operator/internal/webhookhandler"
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

type languageInstrumentations struct {
	Java   *v1alpha1.Instrumentation
	NodeJS *v1alpha1.Instrumentation
	Python *v1alpha1.Instrumentation
	DotNet *v1alpha1.Instrumentation
	Php    *v1alpha1.Instrumentation
	Go     *v1alpha1.Instrumentation
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

	insts := languageInstrumentations{}

	// We bail out if any annotation fails to process.

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectJava); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	insts.Java = inst

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectNodeJS); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	insts.NodeJS = inst

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectPython); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	insts.Python = inst

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectDotNet); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	insts.DotNet = inst

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectPhp); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "failed to select a New Relic Instrumentation instance for this pod")
		return pod, err
	}
	insts.Php = inst

	if inst, err = pm.getInstrumentationInstance(ctx, ns, pod, annotationInjectGo); err != nil {
		// we still allow the pod to be created, but we log a message to the operator's logs
		logger.Error(err, "support for Go auto instrumentation is not enabled")
		return pod, err
	}
	insts.Go = inst

	if insts.Java == nil && insts.NodeJS == nil && insts.Python == nil && insts.DotNet == nil && insts.Php == nil && insts.Go == nil {
		logger.V(1).Info("annotation not present in deployment, skipping instrumentation injection")
		return pod, nil
	}

	// We retrieve the annotation for podname
	var targetContainers = annotationValue(ns.ObjectMeta, pod.ObjectMeta, annotationInjectContainerName)

	// once it's been determined that instrumentation is desired, none exists yet, and we know which instance it should talk to,
	// we should inject the instrumentation.
	modifiedPod := pod
	for _, currentContainer := range strings.Split(targetContainers, ",") {
		modifiedPod = pm.sdkInjector.inject(ctx, insts, ns, modifiedPod, strings.TrimSpace(currentContainer))
	}

	return modifiedPod, nil
}

func (pm *instPodMutator) getInstrumentationInstance(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, instAnnotation string) (*v1alpha1.Instrumentation, error) {
	instValue := annotationValue(ns.ObjectMeta, pod.ObjectMeta, instAnnotation)

	if len(instValue) == 0 || strings.EqualFold(instValue, "false") {
		return nil, nil
	}

	if strings.EqualFold(instValue, "true") {
		return pm.selectInstrumentationInstanceFromNamespace(ctx, ns)
	}

	var instNamespacedName types.NamespacedName
	if instNamespace, instName, namespaced := strings.Cut(instValue, "/"); namespaced {
		instNamespacedName = types.NamespacedName{Name: instName, Namespace: instNamespace}
	} else {
		instNamespacedName = types.NamespacedName{Name: instValue, Namespace: ns.Name}
	}

	nrInst := &v1alpha1.Instrumentation{}
	err := pm.Client.Get(ctx, instNamespacedName, nrInst)
	if err != nil {
		return nil, err
	}

	return nrInst, nil
}

func (pm *instPodMutator) selectInstrumentationInstanceFromNamespace(ctx context.Context, ns corev1.Namespace) (*v1alpha1.Instrumentation, error) {
	var nrInsts v1alpha1.InstrumentationList
	if err := pm.Client.List(ctx, &nrInsts, client.InNamespace(ns.Name)); err != nil {
		return nil, err
	}

	switch s := len(nrInsts.Items); {
	case s == 0:
		return nil, errNoInstancesAvailable
	case s > 1:
		return nil, errMultipleInstancesPossible
	default:
		return &nrInsts.Items[0], nil
	}
}
