package apm

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

type baseInjector struct {
	client client.Client
	lang   string
}

func (i *baseInjector) ConfigureClient(client client.Client) {
	i.client = client
}

func (i *baseInjector) Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.lang {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	if inst.Spec.LicenseKeySecret == "" {
		return false
	}
	return true
}

func (i *baseInjector) Language() string {
	return i.lang
}

func (i *baseInjector) setContainerEnvAppName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod, container *corev1.Container) error {
	if idx := getIndexOfEnv(container.Env, EnvNewRelicAppName); idx == -1 {
		name, err := i.getRootResourceName(ctx, ns, pod)
		if err != nil {
			return fmt.Errorf("failed to get root resource name for pod > %w", err)
		}
		if name == "" {
			name = container.Name
		}
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvNewRelicAppName,
			Value: name,
		})
	}
	return nil
}

func (i *baseInjector) getRootResourceName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod) (string, error) {
	for _, owner := range pod.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "deployment", "statefulset", "cronjob", "daemonset":
			return owner.Name, nil
		}
	}
	checkError := func(err error) bool { return apierrors.IsNotFound(err) }
	backOff := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 1.5, Jitter: 0.1, Steps: 20, Cap: 2 * time.Second}
	for _, owner := range pod.OwnerReferences {
		ownerName := types.NamespacedName{Namespace: ns.Name, Name: owner.Name}
		switch strings.ToLower(owner.Kind) {
		case "job":
			obj := batchv1.Job{}
			if err := retry.OnError(backOff, checkError, func() error { return i.client.Get(ctx, ownerName, &obj) }); err != nil {
				return "", fmt.Errorf("failed to get parent name > %w", err)
			}
			for _, parentOwner := range obj.OwnerReferences {
				if strings.ToLower(parentOwner.Kind) == "cronjob" {
					return parentOwner.Name, nil
				}
			}
			return owner.Name, nil
		case "replicaset":
			obj := appsv1.ReplicaSet{}
			if err := retry.OnError(backOff, checkError, func() error { return i.client.Get(ctx, ownerName, &obj) }); err != nil {
				return "", fmt.Errorf("failed to get parent name > %w", err)
			}
			for _, parentOwner := range obj.OwnerReferences {
				if strings.ToLower(parentOwner.Kind) == "deployment" {
					return parentOwner.Name, nil
				}
			}
			return owner.Name, nil
		}
	}
	return pod.Name, nil
}
