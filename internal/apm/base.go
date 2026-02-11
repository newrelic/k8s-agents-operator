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

// NewRelicAppNameAnnotation is the annotation used to set the same New Relic application name to all the pods instrumented
// by the same Instrumentation resource.
// This does not have precedence over the application name set at the environment variable level, which will be used if defined.
const NewRelicAppNameAnnotation = "newrelic.com/app-name"

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

func (i *baseInjector) setContainerEnvAppName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod, container *corev1.Container, inst current.Instrumentation) error {
	// if the EnvNewRelicAppName is already set in the container, we do not override it
	if idx := getIndexOfEnv(container.Env, EnvNewRelicAppName); idx != -1 {
		return nil
	}

	name, err := i.retrieveAppName(ctx, ns, pod, container, inst)
	if err != nil {
		return fmt.Errorf("failed to retrieve application name > %w", err)
	}

	container.Env = append(container.Env, corev1.EnvVar{
		Name:  EnvNewRelicAppName,
		Value: name,
	})
	return nil
}

// retrieveAppName determines the application name to be used for a given pod and container when and environment variable is not already specified.
// It first checks if the application name is specified in the instrumentation annotations.
// If not, it attempts to derive the application name from the pod's owner resources (like Deployment, StatefulSet, etc.).
// If it cannot determine a name from the owners, it defaults to using the pod's name as the application name, falling
// back to the container name if empty.
func (i *baseInjector) retrieveAppName(ctx context.Context, ns *corev1.Namespace, pod *corev1.Pod, container *corev1.Container, inst current.Instrumentation) (string, error) {
	if appName, ok := inst.Annotations[NewRelicAppNameAnnotation]; ok {
		return appName, nil
	}

	if rootResourceName, err := i.getRootResourceName(ctx, ns, pod); err != nil {
		return "", fmt.Errorf("failed to get root resource name for pod > %w", err)
	} else if rootResourceName != "" {
		return rootResourceName, nil
	}

	return container.Name, nil
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
