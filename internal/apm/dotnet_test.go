package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

func TestDotnetInjector_Inject(t *testing.T) {
	vtrue := true
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		inst           current.Instrumentation
		expectedPod    corev1.Pod
		expectedErrStr string
		containerNames []string
		useNewMethod   bool
	}{
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetCoreClrEnableProfiling, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: CORECLR_ENABLE_PROFILING",
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env CORECLR_ENABLE_PROFILING already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetCoreClrEnableProfiling, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetCoreClrProfiler, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER_PATH already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetCoreClrProfilerPath, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env CORECLR_NEWRELIC_HOME already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetNewrelicHome, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID"}},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			inst:           current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test"},
			}}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Env: []corev1.EnvVar{
							{Name: "CORECLR_ENABLE_PROFILING", Value: "1"},
							{Name: "CORECLR_PROFILER", Value: "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
							{Name: "CORECLR_PROFILER_PATH", Value: "/nri-dotnet--test/libNewRelicProfiler.so"},
							{Name: "CORECLR_NEWRELIC_HOME", Value: "/nri-dotnet--test"},
							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "/nri-dotnet--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-dotnet--test",
						Command:      []string{"cp", "-a", "/instrumentation/.", "/nri-dotnet--test/"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "/nri-dotnet--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-dotnet--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				}},
			inst: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet"}, LicenseKeySecret: "newrelic-key-secret"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &DotnetInjector{baseInjector{lang: "dotnet"}}
			// inject multiple times to assert that it's idempotent
			var err error
			var actualPod corev1.Pod
			testPod := test.pod
		loop:
			for ic := 0; ic < 3; ic++ {
				if !i.Accepts(test.inst, test.ns, testPod) {
					actualPod = testPod
					continue
				}
				if test.useNewMethod {
					var containerNames []string
					if len(test.containerNames) == 0 && len(test.pod.Spec.Containers) > 0 {
						containerNames = append(containerNames, test.pod.Spec.Containers[0].Name)
					} else {
						containerNames = append(containerNames, test.containerNames...)
					}
					for _, containerName := range containerNames {
						actualPod, err = i.InjectContainer(ctx, test.inst, test.ns, testPod, containerName)
						if err != nil {
							break loop
						}
						testPod = actualPod
					}
				} else {
					actualPod, err = i.Inject(ctx, test.inst, test.ns, testPod)
				}
				if err != nil {
					break
				}
				testPod = actualPod
			}
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			require.Equal(t, test.expectedErrStr, errStr)
			if diff := cmp.Diff(test.expectedPod, actualPod); diff != "" {
				assert.Fail(t, diff)
			}
		})
	}
}
