package v1beta2

import (
	"github.com/google/go-cmp/cmp"
	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestConvertTo(t *testing.T) {
	src := Instrumentation{
		Spec: InstrumentationSpec{
			PodLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"c"},
					},
				},
			},
			NamespaceLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"c": "d",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "d",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"e"},
					},
				},
			},
			ContainerSelector: ContainerSelector{
				NamesFromPodAnnotation: "f",
				EnvSelector: EnvSelector{
					MatchEnvs: map[string]string{
						"g": "h",
					},
					MatchExpressions: []EnvSelectorRequirement{
						{
							Key:      "h",
							Operator: EnvSelectorOpIn,
							Values:   []string{"i"},
						},
					},
				},
				ImageSelector: ImageSelector{
					MatchImages: map[string]string{},
					MatchExpressions: []ImageSelectorRequirement{
						{
							Key:      ImageSelectorKeyUrl,
							Operator: ImageSelectorOpIn,
							Values:   []string{"j"},
						},
					},
				},
				NameSelector: NameSelector{
					MatchNames: map[string]string{
						"k": "l",
					},
					MatchExpressions: []NameSelectorRequirement{
						{
							Key:      "l",
							Operator: NameSelectorOpIn,
							Values:   []string{"m"},
						},
					},
				},
			},
			Agent: Agent{
				Language:        "test-language",
				Image:           "test-image",
				ImagePullPolicy: corev1.PullNever,
				VolumeSizeLimit: resource.NewQuantity(88, resource.BinarySI),
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			HealthAgent: HealthAgent{
				Image:           "test-image2",
				ImagePullPolicy: corev1.PullAlways,
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			LicenseKeySecret: "test-license-key",
			AgentConfigMap:   "test-agent-configmap",
		},
		Status: InstrumentationStatus{
			PodsMatching:  1,
			PodsHealthy:   2,
			PodsOutdated:  3,
			PodsInjected:  4,
			PodsUnhealthy: 5,
			PodsNotReady:  6,
			UnhealthyPodsErrors: []UnhealthyPodError{
				{
					Pod:       "def",
					LastError: "ghi",
				},
			},
			LastUpdated:     metav1.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			ObservedVersion: "abc",
		},
	}
	dst := current.Instrumentation{}
	expectedInstrumentation := current.Instrumentation{
		Spec: current.InstrumentationSpec{
			PodLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"c"},
					},
				},
			},
			NamespaceLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"c": "d",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "d",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"e"},
					},
				},
			},
			ContainerSelector: current.ContainerSelector{
				NamesFromPodAnnotation: "f",
				EnvSelector: current.EnvSelector{
					MatchEnvs: map[string]string{
						"g": "h",
					},
					MatchExpressions: []current.EnvSelectorRequirement{
						{
							Key:      "h",
							Operator: current.EnvSelectorOpIn,
							Values:   []string{"i"},
						},
					},
				},
				ImageSelector: current.ImageSelector{
					MatchImages: map[string]string{},
					MatchExpressions: []current.ImageSelectorRequirement{
						{
							Key:      current.ImageSelectorKeyUrl,
							Operator: current.ImageSelectorOpIn,
							Values:   []string{"j"},
						},
					},
				},
				NameSelector: current.NameSelector{
					MatchNames: map[string]string{
						"k": "l",
					},
					MatchExpressions: []current.NameSelectorRequirement{
						{
							Key:      "l",
							Operator: current.NameSelectorOpIn,
							Values:   []string{"m"},
						},
					},
				},
			},
			Agent: current.Agent{
				Language:        "test-language",
				Image:           "test-image",
				ImagePullPolicy: corev1.PullNever,
				VolumeSizeLimit: resource.NewQuantity(88, resource.BinarySI),
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			HealthAgent: current.HealthAgent{
				Image:           "test-image2",
				ImagePullPolicy: corev1.PullAlways,
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			LicenseKeySecret: "test-license-key",
			AgentConfigMap:   "test-agent-configmap",
		},

		Status: current.InstrumentationStatus{
			PodsMatching:  1,
			PodsHealthy:   2,
			PodsOutdated:  3,
			PodsInjected:  4,
			PodsUnhealthy: 5,
			PodsNotReady:  6,
			UnhealthyPodsErrors: []current.UnhealthyPodError{
				{
					Pod:       "def",
					LastError: "ghi",
				},
			},
			LastUpdated:     metav1.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			ObservedVersion: "abc",
		},
	}
	err := src.ConvertTo(&dst)
	assert.NoError(t, err)
	if diff := cmp.Diff(expectedInstrumentation, dst); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestConvertFrom(t *testing.T) {
	src := current.Instrumentation{
		Spec: current.InstrumentationSpec{
			PodLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"c"},
					},
				},
			},
			NamespaceLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"c": "d",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "d",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"e"},
					},
				},
			},
			ContainerSelector: current.ContainerSelector{
				NamesFromPodAnnotation: "f",
				EnvSelector: current.EnvSelector{
					MatchEnvs: map[string]string{
						"g": "h",
					},
					MatchExpressions: []current.EnvSelectorRequirement{
						{
							Key:      "h",
							Operator: current.EnvSelectorOpIn,
							Values:   []string{"i"},
						},
					},
				},
				ImageSelector: current.ImageSelector{
					MatchImages: map[string]string{},
					MatchExpressions: []current.ImageSelectorRequirement{
						{
							Key:      current.ImageSelectorKeyUrl,
							Operator: current.ImageSelectorOpIn,
							Values:   []string{"j"},
						},
					},
				},
				NameSelector: current.NameSelector{
					MatchNames: map[string]string{
						"k": "l",
					},
					MatchExpressions: []current.NameSelectorRequirement{
						{
							Key:      "l",
							Operator: current.NameSelectorOpIn,
							Values:   []string{"m"},
						},
					},
				},
			},
			Agent: current.Agent{
				Language:        "test-language",
				Image:           "test-image",
				ImagePullPolicy: corev1.PullNever,
				VolumeSizeLimit: resource.NewQuantity(88, resource.BinarySI),
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			HealthAgent: current.HealthAgent{
				Image:           "test-image2",
				ImagePullPolicy: corev1.PullAlways,
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			LicenseKeySecret: "test-license-key",
			AgentConfigMap:   "test-agent-configmap",
		},
		Status: current.InstrumentationStatus{
			PodsMatching:  1,
			PodsHealthy:   2,
			PodsOutdated:  3,
			PodsInjected:  4,
			PodsUnhealthy: 5,
			PodsNotReady:  6,
			UnhealthyPodsErrors: []current.UnhealthyPodError{
				{
					Pod:       "def",
					LastError: "ghi",
				},
			},
			LastUpdated:     metav1.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			ObservedVersion: "abc",
			EntityGUIDs:     []string{"this cant be copied"},
		},
	}
	dst := Instrumentation{}
	expectedInstrumentation := Instrumentation{
		Spec: InstrumentationSpec{
			PodLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "b",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"c"},
					},
				},
			},
			NamespaceLabelSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"c": "d",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "d",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"e"},
					},
				},
			},
			ContainerSelector: ContainerSelector{
				NamesFromPodAnnotation: "f",
				EnvSelector: EnvSelector{
					MatchEnvs: map[string]string{
						"g": "h",
					},
					MatchExpressions: []EnvSelectorRequirement{
						{
							Key:      "h",
							Operator: EnvSelectorOpIn,
							Values:   []string{"i"},
						},
					},
				},
				ImageSelector: ImageSelector{
					MatchImages: map[string]string{},
					MatchExpressions: []ImageSelectorRequirement{
						{
							Key:      ImageSelectorKeyUrl,
							Operator: ImageSelectorOpIn,
							Values:   []string{"j"},
						},
					},
				},
				NameSelector: NameSelector{
					MatchNames: map[string]string{
						"k": "l",
					},
					MatchExpressions: []NameSelectorRequirement{
						{
							Key:      "l",
							Operator: NameSelectorOpIn,
							Values:   []string{"m"},
						},
					},
				},
			},
			Agent: Agent{
				Language:        "test-language",
				Image:           "test-image",
				ImagePullPolicy: corev1.PullNever,
				VolumeSizeLimit: resource.NewQuantity(88, resource.BinarySI),
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			HealthAgent: HealthAgent{
				Image:           "test-image2",
				ImagePullPolicy: corev1.PullAlways,
				Env:             []corev1.EnvVar{},
				Resources:       corev1.ResourceRequirements{},
				SecurityContext: &corev1.SecurityContext{},
			},
			LicenseKeySecret: "test-license-key",
			AgentConfigMap:   "test-agent-configmap",
		},
		Status: InstrumentationStatus{
			PodsMatching:  1,
			PodsHealthy:   2,
			PodsOutdated:  3,
			PodsInjected:  4,
			PodsUnhealthy: 5,
			PodsNotReady:  6,
			UnhealthyPodsErrors: []UnhealthyPodError{
				{
					Pod:       "def",
					LastError: "ghi",
				},
			},
			LastUpdated:     metav1.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			ObservedVersion: "abc",
		},
	}
	err := dst.ConvertFrom(&src)
	assert.NoError(t, err)
	if diff := cmp.Diff(expectedInstrumentation, dst); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
