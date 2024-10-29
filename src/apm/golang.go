package apm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unsafe"

	semconv "go.opentelemetry.io/otel/semconv/v1.5.0"
	corev1 "k8s.io/api/core/v1"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
)

const (
	annotationGoExecPath  = "instrumentation.opentelemetry.io/otel-go-auto-target-exe"
	envOtelTargetExe      = "OTEL_GO_AUTO_TARGET_EXE"
	kernelDebugVolumeName = "kernel-debug"
	kernelDebugVolumePath = "/sys/kernel/debug"
	sideCarName           = "opentelemetry-auto-instrumentation"
)

const (
	EnvOTELServiceName          = "OTEL_SERVICE_NAME"
	EnvOTELExporterOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvOTELResourceAttrs        = "OTEL_RESOURCE_ATTRIBUTES"
	EnvOTELPropagators          = "OTEL_PROPAGATORS"
	EnvOTELTracesSampler        = "OTEL_TRACES_SAMPLER"
	EnvOTELTracesSamplerArg     = "OTEL_TRACES_SAMPLER_ARG"

	EnvPodName  = "OTEL_RESOURCE_ATTRIBUTES_POD_NAME"
	EnvPodUID   = "OTEL_RESOURCE_ATTRIBUTES_POD_UID"
	EnvNodeName = "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME"
)

var _ Injector = (*GoInjector)(nil)

func init() {
	DefaultInjectorRegistry.MustRegister(&GoInjector{})
}

type GoInjector struct {
	baseInjector
}

func (i *GoInjector) Language() string {
	return "go"
}

func (i *GoInjector) acceptable(inst v1alpha2.Instrumentation, pod corev1.Pod) bool {
	if inst.Spec.Agent.Language != i.Language() {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	return true
}

func (i *GoInjector) Inject(ctx context.Context, inst v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error) {
	if !i.acceptable(inst, pod) {
		return pod, nil
	}
	if err := i.validate(inst); err != nil {
		return pod, err
	}

	// skip instrumentation if share process namespaces is explicitly disabled
	if pod.Spec.ShareProcessNamespace != nil && !*pod.Spec.ShareProcessNamespace {
		return pod, fmt.Errorf("shared process namespace has been explicitly disabled")
	}

	vtrue := true
	vzero := int64(0)
	pod.Spec.ShareProcessNamespace = &vtrue

	goAgent := corev1.Container{
		Name:      sideCarName,
		Image:     inst.Spec.Agent.Image,
		Resources: inst.Spec.Agent.Resources,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  &vzero,
			Privileged: &vtrue,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				MountPath: "/sys/kernel/debug",
				Name:      kernelDebugVolumeName,
			},
		},
	}

	// Annotation takes precedence for OTEL_GO_AUTO_TARGET_EXE
	execPath, ok := pod.Annotations[annotationGoExecPath]
	if ok {
		goAgent.Env = append(goAgent.Env, corev1.EnvVar{
			Name:  envOtelTargetExe,
			Value: execPath,
		})
	}

	// Inject Go instrumentation spec env vars.
	// For Go, env vars must be added to the agent contain
	for _, env := range inst.Spec.Agent.Env {
		idx := getIndexOfEnv(goAgent.Env, env.Name)
		if idx == -1 {
			goAgent.Env = append(goAgent.Env, env)
		}
	}

	pod.Spec.Containers = append(pod.Spec.Containers, goAgent)
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: kernelDebugVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: kernelDebugVolumePath,
			},
		},
	})

	lastIndex := len(pod.Spec.Containers) - 1
	pod = i.injectEnvVar(inst, pod, lastIndex)
	pod = i.injectCommonSDKConfig(ctx, inst, ns, pod, lastIndex, 0)

	return pod, nil
}

func (i *GoInjector) injectEnvVar(newrelic v1alpha2.Instrumentation, pod corev1.Pod, index int) corev1.Pod {
	container := &pod.Spec.Containers[index]
	for _, env := range newrelic.Spec.Agent.Env {
		idx := getIndexOfEnv(container.Env, env.Name)
		if idx == -1 {
			container.Env = append(container.Env, env)
		}
		//@todo: should we update this if it exists?
	}
	return pod
}

// injectCommonSDKConfig adds common SDK configuration environment variables to the necessary pod
// agentIndex represents the index of the pod the needs the env vars to instrument the application.
// appIndex represents the index of the pod the will produce the telemetry.
// When the pod handling the instrumentation is the same as the pod producing the telemetry agentIndex
// and appIndex should be the same value.  This is true for dotnet, java, nodejs, python, and ruby instrumentations.
// Go requires the agent to be a different container in the pod, so the agentIndex should represent this new sidecar
// and appIndex should represent the application being instrumented.
func (i *GoInjector) injectCommonSDKConfig(ctx context.Context, newrelic v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod, agentIndex int, appIndex int) corev1.Pod {
	container := &pod.Spec.Containers[agentIndex]
	resourceMap := i.createResourceMap(ctx, newrelic.Spec.Resource, ns, pod, appIndex)
	idx := getIndexOfEnv(container.Env, EnvOTELServiceName)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvOTELServiceName,
			Value: chooseServiceName(pod, resourceMap, appIndex),
		})
	}
	if newrelic.Spec.Exporter.Endpoint != "" {
		idx = getIndexOfEnv(container.Env, EnvOTELExporterOTLPEndpoint)
		if idx == -1 {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  EnvOTELExporterOTLPEndpoint,
				Value: newrelic.Spec.Endpoint,
			})
		}
	}

	// Some attributes might be empty, we should get them via k8s downward API
	if resourceMap[string(semconv.K8SPodNameKey)] == "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: EnvPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		})
		resourceMap[string(semconv.K8SPodNameKey)] = fmt.Sprintf("$(%s)", EnvPodName)
	}
	if newrelic.Spec.Resource.AddK8sUIDAttributes {
		if resourceMap[string(semconv.K8SPodUIDKey)] == "" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name: EnvPodUID,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.uid",
					},
				},
			})
			resourceMap[string(semconv.K8SPodUIDKey)] = fmt.Sprintf("$(%s)", EnvPodUID)
		}
	}

	idx = getIndexOfEnv(container.Env, EnvOTELResourceAttrs)
	if idx == -1 || !strings.Contains(container.Env[idx].Value, string(semconv.ServiceVersionKey)) {
		vsn := chooseServiceVersion(pod, appIndex)
		if vsn != "" {
			resourceMap[string(semconv.ServiceVersionKey)] = vsn
		}
	}

	if resourceMap[string(semconv.K8SNodeNameKey)] == "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: EnvNodeName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		})
		resourceMap[string(semconv.K8SNodeNameKey)] = fmt.Sprintf("$(%s)", EnvNodeName)
	}

	idx = getIndexOfEnv(container.Env, EnvOTELResourceAttrs)
	resStr := resourceMapToStr(resourceMap)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvOTELResourceAttrs,
			Value: resStr,
		})
	} else {
		if !strings.HasSuffix(container.Env[idx].Value, ",") {
			resStr = "," + resStr
		}
		container.Env[idx].Value += resStr
	}

	idx = getIndexOfEnv(container.Env, EnvOTELPropagators)
	if idx == -1 && len(newrelic.Spec.Propagators) > 0 {
		propagators := *(*[]string)((unsafe.Pointer(&newrelic.Spec.Propagators)))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  EnvOTELPropagators,
			Value: strings.Join(propagators, ","),
		})
	}

	idx = getIndexOfEnv(container.Env, EnvOTELTracesSampler)
	// configure sampler only if it is configured in the CR
	if idx == -1 && newrelic.Spec.Sampler.Type != "" {
		idxSamplerArg := getIndexOfEnv(container.Env, EnvOTELTracesSamplerArg)
		if idxSamplerArg == -1 {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  EnvOTELTracesSampler,
				Value: string(newrelic.Spec.Sampler.Type),
			})
			if newrelic.Spec.Sampler.Argument != "" {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  EnvOTELTracesSamplerArg,
					Value: newrelic.Spec.Sampler.Argument,
				})
			}
		}
	}

	// Move OTEL_RESOURCE_ATTRIBUTES to last position on env list.
	// When OTEL_RESOURCE_ATTRIBUTES environment variable uses other env vars
	// as attributes value they have to be configured before.
	// It is mandatory to set right order to avoid attributes with value
	// pointing to the name of used environment variable instead of its value.
	idx = getIndexOfEnv(container.Env, EnvOTELResourceAttrs)
	envs := moveEnvToListEnd(container.Env, idx)
	container.Env = envs

	return pod
}

func moveEnvToListEnd(envs []corev1.EnvVar, idx int) []corev1.EnvVar {
	if idx >= 0 && idx < len(envs) {
		envToMove := envs[idx]
		envs = append(envs[:idx], envs[idx+1:]...)
		envs = append(envs, envToMove)
	}

	return envs
}

func resourceMapToStr(res map[string]string) string {
	keys := make([]string, 0, len(res))
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var str = ""
	for _, k := range keys {
		if str != "" {
			str += ","
		}
		str += fmt.Sprintf("%s=%s", k, res[k])
	}

	return str
}

// obtains version by splitting image string on ":" and extracting final element from resulting array.
func chooseServiceVersion(pod corev1.Pod, index int) string {
	parts := strings.Split(pod.Spec.Containers[index].Image, ":")
	tag := parts[len(parts)-1]
	//guard statement to handle case where image name has a port number
	if strings.Contains(tag, "/") {
		return ""
	}
	return tag
}
