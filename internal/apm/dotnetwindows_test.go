package apm

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

func TestDotnetWindows2022Injector_Inject(t *testing.T) {
	vtrue := true
	vfalse := false
	type mutation struct {
		containerName   string
		instrumentation current.Instrumentation
	}
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		expectedPod    corev1.Pod
		expectedErrStr string
		mutations      []mutation
	}{
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrEnableProfiling, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: COR_ENABLE_PROFILING",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_ENABLE_PROFILING already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrEnableProfiling, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_PROFILER already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrProfiler, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_PROFILER_PATH already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrProfilerPath, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEWRELIC_HOME already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkNewrelicHome, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrEnableProfiling, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: CORECLR_ENABLE_PROFILING",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_ENABLE_PROFILING already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrEnableProfiling, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrProfiler, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER_PATH already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrProfilerPath, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_NEWRELIC_HOME already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreNewrelicHome, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}}},
			},
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
							{Name: "NEWRELIC_LOG_DIRECTORY", Value: "c:\\nri-dotnet--test\\Logs"},
							{Name: "NEWRELIC_PROFILER_LOG_DIRECTORY", Value: "c:\\nri-dotnet--test\\Logs"},

							{Name: "COR_ENABLE_PROFILING", Value: "1"},
							{Name: "COR_PROFILER", Value: "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"},
							{Name: "COR_PROFILER_PATH", Value: "c:\\nri-dotnet--test\\netframework\\NewRelic.Profiler.dll"},
							{Name: "NEWRELIC_HOME", Value: "c:\\nri-dotnet--test\\netframework"},

							{Name: "CORECLR_ENABLE_PROFILING", Value: "1"},
							{Name: "CORECLR_PROFILER", Value: "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
							{Name: "CORECLR_PROFILER_PATH", Value: "c:\\nri-dotnet--test\\netcore\\NewRelic.Profiler.dll"},
							{Name: "CORECLR_NEWRELIC_HOME", Value: "c:\\nri-dotnet--test\\netcore"},

							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "c:\\nri-dotnet--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-dotnet--test",
						Command:      []string{"cmd", "/C", "xcopy C:\\instrumentation c:\\nri-dotnet--test /E /I /H /Y /F"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "c:\\nri-dotnet--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-dotnet--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2022"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init"},
				},
				Containers: []corev1.Container{
					{Name: "app"},
				},
			}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "nri-dotnet--init",
							Command: []string{"cmd", "/C", "xcopy C:\\instrumentation c:\\nri-dotnet--init /E /I /H /Y /F"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("37Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: &vtrue,
								Privileged:   &vfalse,
							},
							ImagePullPolicy: corev1.PullAlways,
							VolumeMounts:    []corev1.VolumeMount{{Name: "nri-dotnet--init", MountPath: "c:\\nri-dotnet--init"}},
						},
						{
							Name: "init",
							Env: []corev1.EnvVar{
								{Name: "NEWRELIC_LOG_DIRECTORY", Value: "c:\\nri-dotnet--init\\Logs"},
								{Name: "NEWRELIC_PROFILER_LOG_DIRECTORY", Value: "c:\\nri-dotnet--init\\Logs"},

								{Name: "COR_ENABLE_PROFILING", Value: "1"},
								{Name: "COR_PROFILER", Value: "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"},
								{Name: "COR_PROFILER_PATH", Value: "c:\\nri-dotnet--init\\netframework\\NewRelic.Profiler.dll"},
								{Name: "NEWRELIC_HOME", Value: "c:\\nri-dotnet--init\\netframework"},

								{Name: "CORECLR_ENABLE_PROFILING", Value: "1"},
								{Name: "CORECLR_PROFILER", Value: "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
								{Name: "CORECLR_PROFILER_PATH", Value: "c:\\nri-dotnet--init\\netcore\\NewRelic.Profiler.dll"},
								{Name: "CORECLR_NEWRELIC_HOME", Value: "c:\\nri-dotnet--init\\netcore"},

								{Name: "NEW_RELIC_APP_NAME", Value: "init"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--init", MountPath: "c:\\nri-dotnet--init"}},
						},
					},
					Volumes: []corev1.Volume{{Name: "nri-dotnet--init", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{
					containerName: "init",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Language: "dotnet-windows2022",
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									"memory": resource.MustParse("37Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: &vtrue,
								Privileged:   &vfalse,
							},
							ImagePullPolicy: corev1.PullAlways,
						},
						LicenseKeySecret: "newrelic-key-secret"},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &DotnetWindowsInjector{baseInjector{lang: "dotnet-windows2022"}}
			// inject multiple times to assert that it's idempotent. validate it's correct each time
			var err error
			var actualPod corev1.Pod
			testPod := test.pod

			for ic := 0; ic < 3; ic++ {
			mutLoop:
				for _, mut := range test.mutations {
					if !i.Accepts(mut.instrumentation, test.ns, testPod) {
						continue
					}
					containerName := mut.containerName
					if containerName == "" {
						containerName = testPod.Spec.Containers[0].Name
					}

					actualPod, err = i.InjectContainer(ctx, mut.instrumentation, test.ns, testPod, containerName)
					if err != nil {
						break mutLoop
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
			}
		})
	}
}

func TestDotnetWindows2025Injector_Inject(t *testing.T) {
	vtrue := true
	vfalse := false
	type mutation struct {
		containerName   string
		instrumentation current.Instrumentation
	}
	tests := []struct {
		name           string
		pod            corev1.Pod
		ns             corev1.Namespace
		expectedPod    corev1.Pod
		expectedErrStr string
		mutations      []mutation
	}{
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrEnableProfiling, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: COR_ENABLE_PROFILING",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_ENABLE_PROFILING already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrEnableProfiling, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_PROFILER already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrProfiler, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env COR_PROFILER_PATH already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkClrProfilerPath, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEWRELIC_HOME already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envDotnetFrameworkNewrelicHome, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env already set to ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrEnableProfiling, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: CORECLR_ENABLE_PROFILING",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_ENABLE_PROFILING already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrEnableProfiling, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrProfiler, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_PROFILER_PATH already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreClrProfilerPath, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env CORECLR_NEWRELIC_HOME already set",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: windowsEnvDotnetCoreNewrelicHome, Value: "INVALID"}}},
			}}},
			expectedErrStr: errUnableToConfigureEnv.Error(),
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID"}}},
			},
		},
		{
			name: "a container, instrumentation with env NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION already set using ValueFrom",
			pod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "test", Env: []corev1.EnvVar{{Name: envAgentControlHealthDeliveryLocation, ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "test"}}}}}},
			}}},
			expectedErrStr: "the container defines env var value via ValueFrom, envVar: NEW_RELIC_AGENT_CONTROL_HEALTH_DELIVERY_LOCATION",
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "VALID", HealthAgent: current.HealthAgent{Image: "health"}}}},
			},
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
							{Name: "NEWRELIC_LOG_DIRECTORY", Value: "c:\\nri-dotnet--test\\Logs"},
							{Name: "NEWRELIC_PROFILER_LOG_DIRECTORY", Value: "c:\\nri-dotnet--test\\Logs"},

							{Name: "COR_ENABLE_PROFILING", Value: "1"},
							{Name: "COR_PROFILER", Value: "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"},
							{Name: "COR_PROFILER_PATH", Value: "c:\\nri-dotnet--test\\netframework\\NewRelic.Profiler.dll"},
							{Name: "NEWRELIC_HOME", Value: "c:\\nri-dotnet--test\\netframework"},

							{Name: "CORECLR_ENABLE_PROFILING", Value: "1"},
							{Name: "CORECLR_PROFILER", Value: "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
							{Name: "CORECLR_PROFILER_PATH", Value: "c:\\nri-dotnet--test\\netcore\\NewRelic.Profiler.dll"},
							{Name: "CORECLR_NEWRELIC_HOME", Value: "c:\\nri-dotnet--test\\netcore"},

							{Name: "NEW_RELIC_APP_NAME", Value: "test"},
							{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
							{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
							{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "c:\\nri-dotnet--test"}},
					}},
					InitContainers: []corev1.Container{{
						Name:         "nri-dotnet--test",
						Command:      []string{"cmd", "/C", "xcopy C:\\instrumentation c:\\nri-dotnet--test /E /I /H /Y /F"},
						VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--test", MountPath: "c:\\nri-dotnet--test"}},
					}},
					Volumes: []corev1.Volume{{Name: "nri-dotnet--test", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet-windows2025"}, LicenseKeySecret: "newrelic-key-secret"}}},
			},
		},
		{
			name: "a container, instrumentation",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init"},
				},
				Containers: []corev1.Container{
					{Name: "app"},
				},
			}},
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"newrelic.com/instrumentation-versions": `{"/":"/0"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:    "nri-dotnet--init",
							Command: []string{"cmd", "/C", "xcopy C:\\instrumentation c:\\nri-dotnet--init /E /I /H /Y /F"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("37Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: &vtrue,
								Privileged:   &vfalse,
							},
							ImagePullPolicy: corev1.PullAlways,
							VolumeMounts:    []corev1.VolumeMount{{Name: "nri-dotnet--init", MountPath: "c:\\nri-dotnet--init"}},
						},
						{
							Name: "init",
							Env: []corev1.EnvVar{
								{Name: "NEWRELIC_LOG_DIRECTORY", Value: "c:\\nri-dotnet--init\\Logs"},
								{Name: "NEWRELIC_PROFILER_LOG_DIRECTORY", Value: "c:\\nri-dotnet--init\\Logs"},

								{Name: "COR_ENABLE_PROFILING", Value: "1"},
								{Name: "COR_PROFILER", Value: "{71DA0A04-7777-4EC6-9643-7D28B46A8A41}"},
								{Name: "COR_PROFILER_PATH", Value: "c:\\nri-dotnet--init\\netframework\\NewRelic.Profiler.dll"},
								{Name: "NEWRELIC_HOME", Value: "c:\\nri-dotnet--init\\netframework"},

								{Name: "CORECLR_ENABLE_PROFILING", Value: "1"},
								{Name: "CORECLR_PROFILER", Value: "{36032161-FFC0-4B61-B559-F6C5D41BAE5A}"},
								{Name: "CORECLR_PROFILER_PATH", Value: "c:\\nri-dotnet--init\\netcore\\NewRelic.Profiler.dll"},
								{Name: "CORECLR_NEWRELIC_HOME", Value: "c:\\nri-dotnet--init\\netcore"},

								{Name: "NEW_RELIC_APP_NAME", Value: "init"},
								{Name: "NEW_RELIC_LABELS", Value: "operator:auto-injection"},
								{Name: "NEW_RELIC_K8S_OPERATOR_ENABLED", Value: "true"},
								{Name: "NEW_RELIC_LICENSE_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "newrelic-key-secret"}, Key: "new_relic_license_key", Optional: &vtrue}}},
							},
							VolumeMounts: []corev1.VolumeMount{{Name: "nri-dotnet--init", MountPath: "c:\\nri-dotnet--init"}},
						},
					},
					Volumes: []corev1.Volume{{Name: "nri-dotnet--init", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
			mutations: []mutation{
				{
					containerName: "init",
					instrumentation: current.Instrumentation{Spec: current.InstrumentationSpec{
						Agent: current.Agent{
							Language: "dotnet-windows2025",
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									"memory": resource.MustParse("37Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: &vtrue,
								Privileged:   &vfalse,
							},
							ImagePullPolicy: corev1.PullAlways,
						},
						LicenseKeySecret: "newrelic-key-secret"},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			i := &DotnetWindowsInjector{baseInjector{lang: "dotnet-windows2025"}}
			// inject multiple times to assert that it's idempotent. validate it's correct each time
			var err error
			var actualPod corev1.Pod
			testPod := test.pod

			for ic := 0; ic < 3; ic++ {
			mutLoop:
				for _, mut := range test.mutations {
					if !i.Accepts(mut.instrumentation, test.ns, testPod) {
						continue
					}
					containerName := mut.containerName
					if containerName == "" {
						containerName = testPod.Spec.Containers[0].Name
					}

					actualPod, err = i.InjectContainer(ctx, mut.instrumentation, test.ns, testPod, containerName)
					if err != nil {
						break mutLoop
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
			}
		})
	}
}
