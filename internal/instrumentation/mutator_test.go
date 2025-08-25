package instrumentation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
	"github.com/newrelic/k8s-agents-operator/internal/apm"
	"github.com/newrelic/k8s-agents-operator/internal/selector"
)

type FakeInjector func(ctx context.Context, insts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod

func (fn FakeInjector) Inject(ctx context.Context, insts []*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	if len(pod.Spec.Containers) > 0 {
		return fn(ctx, map[string][]*current.Instrumentation{pod.Spec.Containers[0].Name: insts}, ns, pod)
	}
	return corev1.Pod{}
}

func (fn FakeInjector) InjectContainers(ctx context.Context, insts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	return fn(ctx, insts, ns, pod)
}

var _ SdkContainerInjector = (FakeInjector)(nil)

type FakeSecretReplicator func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error

func (fn FakeSecretReplicator) ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error {
	return fn(ctx, ns, pod, operatorNamespace, []string{secretName})
}

func (fn FakeSecretReplicator) ReplicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error {
	return fn(ctx, ns, pod, operatorNamespace, secretNames)
}

var _ SecretReplicator = (FakeSecretReplicator)(nil)
var _ SecretsReplicator = (FakeSecretReplicator)(nil)

type FakeConfigMapReplicator func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error

func (fn FakeConfigMapReplicator) ReplicateConfigMap(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapName string) error {
	return fn(ctx, ns, pod, operatorNamespace, []string{configMapName})
}

func (fn FakeConfigMapReplicator) ReplicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error {
	return fn(ctx, ns, pod, operatorNamespace, configMapNames)
}

var _ ConfigMapReplicator = (FakeConfigMapReplicator)(nil)
var _ ConfigMapsReplicator = (FakeConfigMapReplicator)(nil)

type FakeInstrumentationLocator func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error)

func (fn FakeInstrumentationLocator) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error) {
	return fn(ctx, ns, pod)
}

type InstrumentationLocatorFn func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error)

func (il InstrumentationLocatorFn) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error) {
	return il(ctx, ns, pod)
}

var _ InstrumentationLocator = (InstrumentationLocatorFn)(nil)

type SdkInjectorFn func(ctx context.Context, insts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod

func (si SdkInjectorFn) InjectContainers(ctx context.Context, insts map[string][]*current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	return si(ctx, insts, ns, pod)
}

var _ SdkContainerInjector = (SdkInjectorFn)(nil)

type SecretReplicatorFn func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error

func (sr SecretReplicatorFn) ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error {
	return sr(ctx, ns, pod, operatorNamespace, []string{secretName})
}

func (sr SecretReplicatorFn) ReplicateSecrets(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretNames []string) error {
	return sr(ctx, ns, pod, operatorNamespace, secretNames)
}

var _ SecretReplicator = (SecretReplicatorFn)(nil)
var _ SecretsReplicator = (SecretReplicatorFn)(nil)

type ConfigMapReplicatorFn func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error

func (sr ConfigMapReplicatorFn) ReplicateConfigMap(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapName string) error {
	return sr(ctx, ns, pod, operatorNamespace, []string{configMapName})
}

func (sr ConfigMapReplicatorFn) ReplicateConfigMaps(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, configMapNames []string) error {
	return sr(ctx, ns, pod, operatorNamespace, configMapNames)
}

var _ ConfigMapReplicator = (ConfigMapReplicatorFn)(nil)
var _ ConfigMapsReplicator = (ConfigMapReplicatorFn)(nil)

//nolint:gocyclo
func TestMutatePod(t *testing.T) {
	var fakeInjector FakeInjector = func(
		ctx context.Context,
		insts map[string][]*current.Instrumentation,
		ns corev1.Namespace,
		pod corev1.Pod,
	) corev1.Pod {
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		for _, containerInsts := range insts {
			for _, inst := range containerInsts {
				if inst.Spec.Agent.Language != "" {
					pod.Annotations["newrelic-"+inst.Spec.Agent.Language] = "true"
				}
			}
		}
		return pod
	}
	var _ SdkContainerInjector = fakeInjector

	var fakeSecretReplicator FakeSecretReplicator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
		operatorNamespace string,
		secretNames []string,
	) error {
		return nil
	}
	var _ SecretsReplicator = fakeSecretReplicator
	var _ SecretReplicator = fakeSecretReplicator

	var fakeConfigMapReplicator FakeConfigMapReplicator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
		operatorNamespace string,
		configMapNames []string,
	) error {
		return nil
	}
	var _ ConfigMapsReplicator = fakeConfigMapReplicator
	var _ ConfigMapReplicator = fakeConfigMapReplicator

	var fakeInstrumentationLocator FakeInstrumentationLocator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
	) ([]*current.Instrumentation, error) {
		return nil, nil
	}
	var fakeInstrumentationLocatorWithDup FakeInstrumentationLocator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
	) ([]*current.Instrumentation, error) {
		return []*current.Instrumentation{
			{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "ruby1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "ruby2"}}},
		}, nil
	}
	var _ InstrumentationLocator = fakeInstrumentationLocator

	tests := []struct {
		enabled     bool
		name        string
		pod         corev1.Pod
		ns          corev1.Namespace
		initInsts   []*current.Instrumentation
		initNs      []*corev1.Namespace
		initSecrets []*corev1.Secret
		operatorNs  string

		expectedPod     corev1.Pod
		expectedSecrets []client.ObjectKey
		expectedErrStr  string

		injector               SdkContainerInjector
		instrumentationLocator InstrumentationLocator
		secretReplicator       SecretsReplicator
		configMapReplicator    ConfigMapsReplicator
	}{
		{
			name: "java injection, true",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns1-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns1-pod"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns1-op"},
					Spec:       current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}},
				},
			},
			initSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "gns1-op"},
					Data:       map[string][]byte{apm.LicenseKey: []byte(base64.RawStdEncoding.EncodeToString([]byte("abc123")))},
				},
			},
			pod:        corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "java-app"}}}},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gns1-pod"}},
			operatorNs: "gns1-op",
			injector:   fakeInjector,
			expectedPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"newrelic-java": "true"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "java-app"}}},
			},
			expectedSecrets: []client.ObjectKey{
				{Name: DefaultLicenseKeySecretName, Namespace: "gns1-op"},
				{Name: DefaultLicenseKeySecretName, Namespace: "gns1-pod"},
			},
		},
		{
			name: "fetch instrumentation error",
			instrumentationLocator: InstrumentationLocatorFn(func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*current.Instrumentation, error) {
				return nil, fmt.Errorf("fetch instrumentation error")
			}),
			pod:            corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:    corev1.Pod{},
			expectedErrStr: "fetch instrumentation error",
		},
		{
			name:                   "no instrumentations",
			instrumentationLocator: fakeInstrumentationLocator,
			pod:                    corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:            corev1.Pod{},
			expectedErrStr:         errNoInstancesAvailable.Error(),
		},
		{
			name:                   "some error when validating language instrumentations",
			instrumentationLocator: fakeInstrumentationLocatorWithDup,
			pod:                    corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:            corev1.Pod{},
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns4-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns4-pod"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns4-op"},
					Spec:       current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}},
				},
			},
			operatorNs:     "gns4-op",
			expectedErrStr: "failed to get language instrumentations for container \"app\": multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: ruby, insts: [a b])",
		},
		{
			name:           "conflicting secret names",
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gns5-pod"}},
			pod:            corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedErrStr: "failed to get license key secret name for container \"app\" with instrumentations names: [\"example-inst-php\" \"example-inst-java\"]: multiple license key secrets possible",
			injector:       fakeInjector,
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns5-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns5-pod"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns5-op"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: "", Agent: current.Agent{Language: "java", Image: "java"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-php", Namespace: "gns5-op"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: "different", Agent: current.Agent{Language: "php", Image: "php"}},
				},
			},
			initSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "gns5-op"},
					Data:       map[string][]byte{apm.LicenseKey: []byte(base64.RawStdEncoding.EncodeToString([]byte("abc123")))},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "different", Namespace: "gns5-op"},
					Data:       map[string][]byte{apm.LicenseKey: []byte(base64.RawStdEncoding.EncodeToString([]byte("def456")))},
				},
			},
			operatorNs: "gns5-op",
		},
		{
			name:        "secret doesn't exist",
			ns:          corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gns6-pod"}},
			pod:         corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"newrelic-java": "true", "newrelic-php": "true"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			injector:    fakeInjector,
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns6-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns6-pod"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns6-op"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: "", Agent: current.Agent{Language: "java", Image: "java"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-php", Namespace: "gns6-op"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: "", Agent: current.Agent{Language: "php", Image: "php"}},
				},
			},
			operatorNs: "gns6-op",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			for _, ns := range test.initNs {
				err := k8sClient.Create(ctx, ns)
				require.NoError(t, err)
				defer func() {
					_ = k8sClient.Delete(ctx, ns)
				}()
			}

			for _, inst := range test.initInsts {
				err := k8sClient.Create(ctx, inst)
				require.NoError(t, err)
				defer func() {
					_ = k8sClient.Delete(ctx, inst)
				}()
			}

			for _, secret := range test.initSecrets {
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
				defer func() {
					_ = k8sClient.Delete(ctx, secret)
				}()
			}

			// by default, we'll use the real implementation
			injector := test.injector
			if injector == nil {
				injectorRegistry := apm.NewInjectorRegistry()
				apmInjectors := []apm.ContainerInjector{
					&apm.DotnetInjector{},
					&apm.JavaInjector{},
					&apm.NodejsInjector{},
					&apm.PhpInjector{},
					&apm.PythonInjector{},
					&apm.RubyInjector{},
				}
				for _, apmInjector := range apmInjectors {
					injectorRegistry.MustRegister(apmInjector)
				}
				injector = NewNewrelicSdkInjector(k8sClient, injectorRegistry)
			}
			instrumentationLocator := test.instrumentationLocator
			if instrumentationLocator == nil {
				instrumentationLocator = NewNewRelicInstrumentationLocator(k8sClient, test.operatorNs)
			}
			//nolint:staticcheck
			var secretReplicator SecretsReplicator = test.secretReplicator
			if secretReplicator == nil {
				secretReplicator = NewNewrelicSecretReplicator(k8sClient)
			}
			configMapReplicator := test.configMapReplicator
			if configMapReplicator == nil {
				configMapReplicator = NewNewrelicConfigMapReplicator(k8sClient)
			}

			mutator := NewMutator(
				k8sClient,
				injector,
				secretReplicator,
				configMapReplicator,
				instrumentationLocator,
				test.operatorNs,
			)
			testPod := test.pod
			resultPod := corev1.Pod{}
			var err error
			for i := 0; i < 1; i++ {
				resultPod, err = mutator.Mutate(ctx, test.ns, testPod)
				if err != nil {
					break
				}
				testPod = resultPod
			}
			if test.expectedErrStr == "" {
				require.NoError(t, err)
				if diff := cmp.Diff(test.expectedPod, resultPod); diff != "" {
					t.Errorf("Unexpected pod diff (-want +got): %s", diff)
				}

				for _, objKey := range test.expectedSecrets {
					var secret corev1.Secret
					err = k8sClient.Get(ctx, objKey, &secret)
					require.NoError(t, err)
				}
			} else {
				actualErrStr := ""
				if err != nil {
					actualErrStr = err.Error()
				}
				assert.Contains(t, actualErrStr, test.expectedErrStr)
				if diff := cmp.Diff(test.expectedPod, resultPod); diff != "" {
					t.Errorf("Unexpected pod diff (-want +got): %s", diff)
				}
			}
		})
	}
}

func TestNewrelicSecretReplicator_ReplicateSecrets(t *testing.T) {
	secretReplicator := NewNewrelicSecretReplicator(k8sClient)

	tests := []struct {
		name           string
		expectedErrStr string
		pod            corev1.Pod
		ns             corev1.Namespace
		operatorNs     string
		secretName     string
		initSecrets    []*corev1.Secret
		initNs         []*corev1.Namespace
	}{
		{
			name:           "no secret",
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "default",
			expectedErrStr: "secrets \"newrelic-key-secret\" not found",
		},
		{
			name: "default secret same ns as operator",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns1"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns1",
		},
		{
			name: "default secret in same ns as pod, no secret in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns2-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns2-pod"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns2-pod"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns2-op",
		},
		{
			name: "default secret in other ns, no secrets in pod or operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns3-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns3-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns3-pod"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns3-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns3-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns3-op",
			expectedErrStr: "secrets \"newrelic-key-secret\" not found",
		},
		{
			name: "default secret in operator ns, no other secrets",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns4-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns4-pod"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns4-op"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns4-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns4-op",
		},
		{
			name: "default secret in operator ns, none in pod ns, secret in other ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns5-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns5-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns5-other"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns5-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns5-other"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns5-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns5-op",
		},
		{
			name: "default secret in operator ns, ns-specific secret in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns6-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns6-pod"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns6-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-secret", Namespace: "ns6-pod"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns6-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns6-op",
		},
		{
			name: "default secret in operator ns, ns-specific secret in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns7-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns7-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns7-other"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns7-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns7-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns7-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns7-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns7-op",
			secretName:     "not-a-normal-newrelic-key-secret",
			expectedErrStr: "secrets \"not-a-normal-newrelic-key-secret\" not found",
		},
		{
			name: "default secret in operator ns, ns-specific secret in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns8-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns8-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns8-other"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns8-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns8-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns8-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-newrelic-key-secret", Namespace: "ns8-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns8-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns8-op",
			secretName:     "not-a-normal-newrelic-key-secret",
			expectedErrStr: "secrets \"not-a-normal-newrelic-key-secret\" not found",
		},
		{
			name: "default secret in operator ns, ns-specific secret in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns9-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns9-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns9-other"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns9-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns9-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns9-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-newrelic-key-secret", Namespace: "ns9-pod"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns9-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns9-op",
			secretName: "not-a-normal-newrelic-key-secret",
		},
		{
			name: "default secret in operator ns, ns-specific secret in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns10-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns10-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns10-other"}},
			},
			initSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns10-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns10-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "ns10-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-newrelic-key-secret", Namespace: "ns10-op"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns10-pod"}},
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs: "ns10-op",
			secretName: "not-a-normal-newrelic-key-secret",
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%d_%s", i, tc.name), func(t *testing.T) {
			ctx := context.Background()

			for _, initNs := range tc.initNs {
				if initNs.Name == "default" || initNs.Name == "" {
					continue
				}
				err := k8sClient.Create(ctx, initNs)
				require.NoError(t, err)
			}
			defer func() {
				for _, initNs := range tc.initNs {
					if initNs.Name == "default" || initNs.Name == "" {
						continue
					}
					err := k8sClient.Delete(ctx, initNs)
					require.NoError(t, err)
				}
			}()

			for _, initSecret := range tc.initSecrets {
				err := k8sClient.Create(ctx, initSecret)
				require.NoError(t, err)
			}
			defer func() {
				for _, initSecret := range tc.initSecrets {
					err := k8sClient.Delete(ctx, initSecret)
					require.NoError(t, err)
				}
			}()

			err := secretReplicator.ReplicateSecrets(ctx, tc.ns, tc.pod, tc.operatorNs, []string{tc.secretName})
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if tc.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", tc.expectedErrStr, errStr)
			}
		})
	}
}

func TestGetLanguageInstrumentations(t *testing.T) {
	tests := []struct {
		name              string
		instrumentations  []*current.Instrumentation
		expectedLangInsts []*current.Instrumentation
		expectedErrStr    string
	}{
		{
			name: "none",
		},
		{
			name: "dotnet",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet", Image: "dotnet"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet", Image: "dotnet"}}},
			},
		},
		{
			name: "go",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "go", Image: "go"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "go", Image: "go"}}},
			},
		},
		{
			name: "java",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
		},
		{
			name: "nodejs",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "nodejs", Image: "nodejs"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "nodejs", Image: "nodejs"}}},
			},
		},
		{
			name: "php",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php", Image: "php"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php", Image: "php"}}},
			},
		},
		{
			name: "python",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python", Image: "python"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python", Image: "python"}}},
			},
		},
		{
			name: "ruby",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "ruby"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "ruby"}}},
			},
		},
		{
			name: "java + php",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php", Image: "php"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
				{Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php", Image: "php"}}},
			},
		},
		{
			name: "java + java, both identical, only return first occurrence",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "1st"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "2st"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
			expectedLangInsts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "1st"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
		},
		{
			name: "java + java, env value different",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "1"}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "0"}}}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: java, insts: [a b])",
		},
		{
			name: "java + java, env name different",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "1"}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "LOGGING", Value: "1"}}}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: java, insts: [a b])",
		},
		{
			name: "java + java, image different",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java-is-great"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java-is-terrible"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: java, insts: [a b])",
		},
		{
			name: "java + java, volume size different",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", VolumeSizeLimit: resource.NewQuantity(2, resource.DecimalSI)}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: java, insts: [a b])",
		},
		{
			name: "java + java, resources different",
			instrumentations: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "java"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select > (lang: java, insts: [a b])",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateInstrumentations(test.instrumentations)
			langInsts := reduceIdenticalInstrumentations(test.instrumentations)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if test.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", test.expectedErrStr, errStr)
			} else if diff := cmp.Diff(test.expectedLangInsts, langInsts); test.expectedErrStr == "" && diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetSecretNameFromInstrumentations(t *testing.T) {
	tests := []struct {
		name             string
		instrumentations []*current.Instrumentation
		expectedErrStr   string
	}{
		{
			name: "none",
		},
		{
			name:             "one, default",
			instrumentations: []*current.Instrumentation{{Spec: current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}}},
		},
		{
			name:             "one, blank",
			instrumentations: []*current.Instrumentation{{Spec: current.InstrumentationSpec{}}},
		},
		{
			name:             "one, other",
			instrumentations: []*current.Instrumentation{{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}}},
		},
		{
			name: "two, one blank, the other the default",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
			},
		},
		{
			name: "three, one something else, one the default, one blank",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
				{Spec: current.InstrumentationSpec{}},
			},
			expectedErrStr: "multiple license key secrets possible",
		},
		{
			name: "two, one blank, the other the something else",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
			expectedErrStr: "multiple license key secrets possible",
		},
		{
			name: "two, one the default, the other the something else",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
			expectedErrStr: "multiple license key secrets possible",
		},
		{
			name: "two, one blank, the other the default",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
			},
		},
		{
			name: "two, both something else",
			instrumentations: []*current.Instrumentation{
				{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}},
				{Spec: current.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLicenseKeySecret(test.instrumentations)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if test.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", test.expectedErrStr, errStr)
			}
		})
	}
}

func TestNewrelicInstrumentationLocator_GetInstrumentations(t *testing.T) {
	tests := []struct {
		name           string
		expectedErrStr string
		initNs         []*corev1.Namespace
		initInsts      []*current.Instrumentation
		ns             corev1.Namespace
		pod            corev1.Pod
		operatorNs     string
		insts          []*current.Instrumentation
	}{
		{
			name: "none",
		},
		{
			name: "not in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "other1"}},
			},
			initInsts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "inst1", Namespace: "other1"}},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other1"}},
			pod:        corev1.Pod{},
			operatorNs: "operator1",
		},
		{
			name: "1 in operator ns, pod selector has error, error is logged and ignored",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator2"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst2", Namespace: "operator2"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{Key: "human", Operator: metav1.LabelSelectorOperator("eat"), Values: []string{"food"}},
							},
						},
					},
				},
			},
			ns:         corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			pod:        corev1.Pod{},
			operatorNs: "operator2",
		},
		{
			name: "1 in operator ns, ns selector has error, error is logged and ignored",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator3"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst3", Namespace: "operator3"},
					Spec: current.InstrumentationSpec{
						NamespaceLabelSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "crowd", Operator: metav1.LabelSelectorOperator("eats"), Values: []string{"foods"}},
						}},
					},
				},
			},
		},
		{
			name: "1 not in operator ns, 1 in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator4-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "operator4-2"}},
			},
			initInsts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "inst4-1", Namespace: "operator4-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "inst4-2", Namespace: "operator4-2"}},
			},
			operatorNs: "operator4-1",
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod4"}},
			insts: []*current.Instrumentation{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst4-1", Namespace: "operator4-1"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
			},
		},
		{
			name: "2 in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator5"}},
			},
			initInsts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "inst5-1", Namespace: "operator5"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "inst5-2", Namespace: "operator5"}},
			},
			operatorNs: "operator5",
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod5"}},
			insts: []*current.Instrumentation{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst5-1", Namespace: "operator5"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst5-2", Namespace: "operator5"},
					Spec:       current.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
			},
		},
		{
			name: "1 matching with pod selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator6"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst6", Namespace: "operator6"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
					},
				},
			},
			operatorNs: "operator6",
			pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod6", Labels: map[string]string{
				"pod-id": "abc1234",
			}}},
			insts: []*current.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst6", Namespace: "operator6"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						LicenseKeySecret: DefaultLicenseKeySecretName,
					},
				},
			},
		},
		{
			name: "1 matching with ns selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator7"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst7", Namespace: "operator7"},
					Spec: current.InstrumentationSpec{
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
					},
				},
			},
			operatorNs: "operator7",
			pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod7", Labels: map[string]string{
				"pod-id": "abc1234",
			}}},
			ns: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{
				"ns-id": "abc1234",
			}}},
			insts: []*current.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst7", Namespace: "operator7"},
					Spec: current.InstrumentationSpec{
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						LicenseKeySecret: DefaultLicenseKeySecretName,
					},
				},
			},
		},
		{
			name: "1 matching with pod selector, but not ns selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator8"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst8", Namespace: "operator8"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
					},
				},
			},
			operatorNs: "operator8",
			pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod8", Labels: map[string]string{
				"pod-id": "abc1234",
			}}},
			ns: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{
				"ns-id": "zxy9876",
			}}},
		},
		{
			name: "1 matching with ns selector, but not pod selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator9"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst9", Namespace: "operator9"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
					},
				},
			},
			operatorNs: "operator9",
			pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod9", Labels: map[string]string{
				"pod-id": "zxy9876",
			}}},
			ns: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{
				"ns-id": "abc1234",
			}}},
		},
		{
			name: "1 matching with ns selector and with pod selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator10"}},
			},
			initInsts: []*current.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst10", Namespace: "operator10"},
					Spec: current.InstrumentationSpec{
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
					},
				},
			},
			operatorNs: "operator10",
			pod: corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod10", Labels: map[string]string{
				"pod-id": "abc1234",
			}}},
			ns: corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default", Labels: map[string]string{
				"ns-id": "abc1234",
			}}},
			insts: []*current.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst10", Namespace: "operator10"},
					Spec: current.InstrumentationSpec{
						NamespaceLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "ns-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						PodLabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "pod-id", Operator: metav1.LabelSelectorOpIn, Values: []string{"abc1234"}}},
						},
						LicenseKeySecret: DefaultLicenseKeySecretName,
					},
				},
			},
		},
	}
	instSorter := func(a, b *current.Instrumentation) bool {
		if a.Namespace > b.Namespace {
			return true
		}
		if a.Name > b.Name {
			return true
		}
		return false
	}
	typeMetaIgnore := cmpopts.IgnoreFields(metav1.TypeMeta{}, "APIVersion")
	objectMetaIgnore := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "UID", "ResourceVersion", "Generation", "CreationTimestamp", "ManagedFields")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			for _, initNs := range test.initNs {
				err := k8sClient.Create(ctx, initNs)
				require.NoError(t, err)
			}
			defer func() {
				for _, initNs := range test.initNs {
					err := k8sClient.Delete(ctx, initNs)
					require.NoError(t, err)
				}
			}()

			for _, initInst := range test.initInsts {
				err := k8sClient.Create(ctx, initInst)
				require.NoError(t, err)
			}
			defer func() {
				for _, initInst := range test.initInsts {
					err := k8sClient.Delete(ctx, initInst)
					require.NoError(t, err)
				}
			}()

			locator := NewNewRelicInstrumentationLocator(k8sClient, test.operatorNs)
			insts, err := locator.GetInstrumentations(ctx, test.ns, test.pod)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if test.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", test.expectedErrStr, errStr)
			}
			if diff := cmp.Diff(test.insts, insts, cmpopts.SortSlices(instSorter), typeMetaIgnore, objectMetaIgnore); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestSetToList(t *testing.T) {
	expectedList := []string{}
	actualList := setToList(map[string]struct{}{})
	if diff := cmp.Diff(expectedList, actualList, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}

	expectedList = []string{"a", "b", "c"}
	actualList = setToList(map[string]struct{}{"c": {}, "b": {}, "a": {}})
	if diff := cmp.Diff(expectedList, actualList, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestIntersectSet(t *testing.T) {
	tests := []struct {
		name        string
		expectedSet map[string]struct{}
		setA        map[string]struct{}
		setB        map[string]struct{}
	}{
		{
			name: "both nil",
		},
		{
			name:        "first nil, second empty",
			expectedSet: nil,
			setB:        map[string]struct{}{},
		},
		{
			name:        "first empty, second nil",
			expectedSet: nil,
			setA:        map[string]struct{}{},
		},
		{
			name:        "both empty",
			expectedSet: nil,
			setA:        map[string]struct{}{},
			setB:        map[string]struct{}{},
		},
		{
			name:        "first with 1 value, second nil",
			expectedSet: nil,
			setA:        map[string]struct{}{"a": {}},
		},
		{
			name:        "first nil, second with 1 value",
			expectedSet: nil,
			setB:        map[string]struct{}{"a": {}},
		},
		{
			name:        "first with 1 value, second empty",
			expectedSet: nil,
			setA:        map[string]struct{}{"a": {}},
			setB:        map[string]struct{}{},
		},
		{
			name:        "first empty, second with 1 value",
			expectedSet: nil,
			setA:        map[string]struct{}{},
			setB:        map[string]struct{}{"a": {}},
		},
		{
			name:        "no intersections",
			expectedSet: nil,
			setA:        map[string]struct{}{"a": {}},
			setB:        map[string]struct{}{"b": {}},
		},
		{
			name:        "intersects only with c",
			expectedSet: map[string]struct{}{"c": {}},
			setA:        map[string]struct{}{"a": {}, "c": {}},
			setB:        map[string]struct{}{"b": {}, "c": {}},
		},
		{
			name:        "intersects only with c and b",
			expectedSet: map[string]struct{}{"c": {}, "b": {}},
			setA:        map[string]struct{}{"a": {}, "b": {}, "c": {}},
			setB:        map[string]struct{}{"b": {}, "c": {}, "d": {}},
		},
		{
			name:        "intersects only with c, but first set is larger",
			expectedSet: map[string]struct{}{"c": {}},
			setA:        map[string]struct{}{"a": {}, "b": {}, "c": {}},
			setB:        map[string]struct{}{"c": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSet := intersectSet(tt.setA, tt.setB)
			if diff := cmp.Diff(tt.expectedSet, actualSet, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetContainerNamesSetFromPod(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "a"},
				{Name: "b"},
			},
			InitContainers: []corev1.Container{
				{Name: "c"},
				{Name: "d"},
			},
		},
	}
	expectedSet := map[string]struct{}{
		"a": {},
		"b": {},
		"c": {},
		"d": {},
	}
	actualSet := getContainerNamesSetFromPod(pod)
	if diff := cmp.Diff(expectedSet, actualSet); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestGetContainerSetByEnvSelector(t *testing.T) {
	selectorNew := func(simpleSelector *selector.SimpleSelector) selector.Selector {
		s, _ := selector.New(simpleSelector)
		return s
	}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "a",
					Env: []corev1.EnvVar{
						{Name: "Q", Value: "q"},
						{Name: "R", Value: "a"},
						{Name: "Z", Value: "z"},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "b",
					Env: []corev1.EnvVar{
						{Name: "Q", Value: "q"},
						{Name: "R", Value: "b"},
						{Name: "X", Value: "x"},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		selector    selector.Selector
		expectedSet map[string]struct{}
	}{
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"X": "x"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"Z": "z"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
		{
			name: "both, exists",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "R",
						Operator: "Exists",
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, not exists",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "A",
						Operator: "NotExists",
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "R",
						Operator: "In",
						Values:   []string{"a", "b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, not equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "R",
						Operator: "!=",
						Values:   []string{"z"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "b, equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "R",
						Operator: "==",
						Values:   []string{"b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "a, not in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "R",
						Operator: "NotIn",
						Values:   []string{"b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSet := getContainerSetByEnvSelector(tt.selector, pod)
			if diff := cmp.Diff(tt.expectedSet, actualSet); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetContainerSetByNameSelector(t *testing.T) {
	selectorNew := func(simpleSelector *selector.SimpleSelector) selector.Selector {
		s, _ := selector.New(simpleSelector)
		return s
	}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "a",
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "b",
				},
			},
		},
	}

	tests := []struct {
		name        string
		selector    selector.Selector
		expectedSet map[string]struct{}
	}{
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"container": "a"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"initContainer": "b"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "both, exists",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "In",
						Values:   []string{"a", "b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, not exists",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "NotIn",
						Values:   []string{"c"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "In",
						Values:   []string{"a", "b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, not equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "!=",
						Values:   []string{"z"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "b, equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "==",
						Values:   []string{"b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "a, not in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "anyContainer",
						Operator: "NotIn",
						Values:   []string{"b"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSet := getContainerSetByNameSelector(tt.selector, pod)
			if diff := cmp.Diff(tt.expectedSet, actualSet); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetContainerSetByImageSelector(t *testing.T) {
	selectorNew := func(simpleSelector *selector.SimpleSelector) selector.Selector {
		s, _ := selector.New(simpleSelector)
		return s
	}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "a",
					Image: "docker.io/library/python:latest",
				},
				{
					Name:  "c",
					Image: "test.local/app:v8.0.1-alpine",
				},
			},
			InitContainers: []corev1.Container{
				{
					Name:  "b",
					Image: "docker.io/library/ruby:v2.4.0",
				},
			},
		},
	}

	tests := []struct {
		name        string
		selector    selector.Selector
		expectedSet map[string]struct{}
	}{
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"url": "docker.io/library/python:latest"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
		{
			name: "exactly 1",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExact:       map[string]string{"url": "docker.io/library/ruby:v2.4.0"},
				MatchExpressions: []selector.SelectorRequirement{},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "both, starts with",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "StartsWith",
						Values:   []string{"docker.io/"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "both, not starts with",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "NotStartsWith",
						Values:   []string{"test.local/"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "all 3, in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "In",
						Values:   []string{"test.local/app:v8.0.1-alpine", "docker.io/library/python:latest", "docker.io/library/ruby:v2.4.0"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}, "c": {}},
		},
		{
			name: "all 3, not equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "!=",
						Values:   []string{"z"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}, "b": {}, "c": {}},
		},
		{
			name: "b, equal",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "==",
						Values:   []string{"docker.io/library/ruby:v2.4.0"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"b": {}},
		},
		{
			name: "a, not in",
			selector: selectorNew(&selector.SimpleSelector{
				MatchExpressions: []selector.SelectorRequirement{
					{
						Key:      "url",
						Operator: "NotIn",
						Values:   []string{"docker.io/library/ruby:v2.4.0", "test.local/app:v8.0.1-alpine"},
					},
				},
			}),
			expectedSet: map[string]struct{}{"a": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSet := getContainerSetByImageSelector(tt.selector, pod)
			if diff := cmp.Diff(tt.expectedSet, actualSet); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetContainerSetByNamesFromPodAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		expectedSet map[string]struct{}
	}{
		{
			name:        "missing annotation, matches everything",
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
		{
			name:        "blank annotation, matches nothing",
			expectedSet: map[string]struct{}{},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"use-these": "",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
		{
			name:        "a only",
			expectedSet: map[string]struct{}{"a": {}},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"use-these": "a",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
		{
			name:        "b only",
			expectedSet: map[string]struct{}{"b": {}},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"use-these": "b",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
		{
			name:        "both",
			expectedSet: map[string]struct{}{"a": {}, "b": {}},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"use-these": "a,b",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
		{
			name:        "none match",
			expectedSet: map[string]struct{}{},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"use-these": "c",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "a",
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "b",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSet := getContainerSetByNamesFromPodAnnotations("use-these", tt.pod)
			if diff := cmp.Diff(tt.expectedSet, actualSet); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetAgentConfigMapsFromInstrumentations(t *testing.T) {
	m := map[string][]*current.Instrumentation{
		"a": {
			{Spec: current.InstrumentationSpec{AgentConfigMap: "z"}},
			{Spec: current.InstrumentationSpec{AgentConfigMap: "x"}},
		},
		"b": {
			{Spec: current.InstrumentationSpec{AgentConfigMap: "y"}},
			{Spec: current.InstrumentationSpec{AgentConfigMap: "z"}},
		},
	}
	s := GetAgentConfigMapsFromInstrumentations(m)
	expectedConfigMapNames := []string{"x", "y", "z"}
	if diff := cmp.Diff(expectedConfigMapNames, s, cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) > 0 })); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestGetConfigMapsFromInstrumentationsAgentEnv(t *testing.T) {
	m := map[string][]*current.Instrumentation{
		"a": {
			{
				Spec: current.InstrumentationSpec{
					Agent: current.Agent{Env: []corev1.EnvVar{
						{Name: "a", Value: "a"},
						{Name: "b", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "a"}}}},
						{Name: "c", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "b"}}}},
					}},
					HealthAgent: current.HealthAgent{Env: []corev1.EnvVar{
						{Name: "d", Value: "d"},
						{Name: "e", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "c"}}}},
						{Name: "f", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "d"}}}},
					}},
				},
			},
		},
		"b": {
			{
				Spec: current.InstrumentationSpec{
					Agent: current.Agent{Env: []corev1.EnvVar{
						{Name: "g", Value: "g"},
						{Name: "h", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "b"}}}},
						{Name: "i", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "c"}}}},
					}},
					HealthAgent: current.HealthAgent{Env: []corev1.EnvVar{
						{Name: "j", Value: "j"},
						{Name: "k", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "d"}}}},
						{Name: "l", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "e"}}}},
					}},
				},
			},
		},
	}
	s := GetConfigMapsFromInstrumentationsAgentEnv(m)
	expectedConfigMapNames := []string{"a", "b", "c", "d", "e"}
	if diff := cmp.Diff(expectedConfigMapNames, s, cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 })); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestGetSecretsFromInstrumentationsAgentEnv(t *testing.T) {

	m := map[string][]*current.Instrumentation{
		"a": {
			{
				Spec: current.InstrumentationSpec{
					Agent: current.Agent{Env: []corev1.EnvVar{
						{Name: "a", Value: "a"},
						{Name: "b", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "a"}}}},
						{Name: "c", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "b"}}}},
					}},
					HealthAgent: current.HealthAgent{Env: []corev1.EnvVar{
						{Name: "d", Value: "d"},
						{Name: "e", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "c"}}}},
						{Name: "f", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "d"}}}},
					}},
				},
			},
		},
		"b": {
			{
				Spec: current.InstrumentationSpec{
					Agent: current.Agent{Env: []corev1.EnvVar{
						{Name: "g", Value: "g"},
						{Name: "h", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "b"}}}},
						{Name: "i", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "c"}}}},
					}},
					HealthAgent: current.HealthAgent{Env: []corev1.EnvVar{
						{Name: "j", Value: "j"},
						{Name: "k", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "d"}}}},
						{Name: "l", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "e"}}}},
					}},
				},
			},
		},
	}
	s := GetSecretsFromInstrumentationsAgentEnv(m)
	expectedConfigMapNames := []string{"a", "b", "c", "d", "e"}
	if diff := cmp.Diff(expectedConfigMapNames, s, cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 })); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestFilterInstrumentations(t *testing.T) {
	ctx := context.Background()
	insts := []*current.Instrumentation{
		{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Spec: current.InstrumentationSpec{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NameSelector: current.NameSelector{MatchExpressions: []current.NameSelectorRequirement{
			{Key: "anyContainer", Operator: "In", Values: []string{"c4", "cb"}},
		}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "c"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{ImageSelector: current.ImageSelector{MatchExpressions: []current.ImageSelectorRequirement{
			{Key: "url", Operator: "StartsWith", Values: []string{"docker.io/library/"}},
		}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "d"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{EnvSelector: current.EnvSelector{MatchExpressions: []current.EnvSelectorRequirement{
			{Key: "PICK_ME", Operator: "Exists"},
		}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "e"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NamesFromPodAnnotation: "use-these"}}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"use-these": "name-annotation-a,name-annotation-b",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "not-being-picked",
					Image: "a",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "name-annotation-a",
					Image: "b",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "c3",
					Image: "c",
					Env: []corev1.EnvVar{
						{Name: "PICK_ME", Value: "1"},
					},
				},
				{
					Name:  "c4",
					Image: "d",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "c5",
					Image: "e",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "c6",
					Image: "docker.io/library/go:latest",
					Env:   []corev1.EnvVar{},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "pick-as-default",
					Image: "g",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "c8",
					Image: "docker.io/library/ruby:latest",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "c9",
					Image: "i",
					Env: []corev1.EnvVar{
						{Name: "PICK_ME", Value: "1"},
					},
				},
				{
					Name:  "name-annotation-b",
					Image: "j",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "cb",
					Image: "k",
					Env:   []corev1.EnvVar{},
				},
				{
					Name:  "cc",
					Image: "l",
					Env:   []corev1.EnvVar{},
				},
			},
		},
	}
	expectedMap := map[string][]*current.Instrumentation{
		"pick-as-default":   {{ObjectMeta: metav1.ObjectMeta{Name: "a"}}},
		"name-annotation-b": {{ObjectMeta: metav1.ObjectMeta{Name: "e"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NamesFromPodAnnotation: "use-these"}}}},
		"name-annotation-a": {{ObjectMeta: metav1.ObjectMeta{Name: "e"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NamesFromPodAnnotation: "use-these"}}}},
		"c9": {{ObjectMeta: metav1.ObjectMeta{Name: "d"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{EnvSelector: current.EnvSelector{MatchExpressions: []current.EnvSelectorRequirement{
			{Key: "PICK_ME", Operator: "Exists"},
		}}}}}},
		"c3": {{ObjectMeta: metav1.ObjectMeta{Name: "d"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{EnvSelector: current.EnvSelector{MatchExpressions: []current.EnvSelectorRequirement{
			{Key: "PICK_ME", Operator: "Exists"},
		}}}}}},
		"c6": {{ObjectMeta: metav1.ObjectMeta{Name: "c"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{ImageSelector: current.ImageSelector{MatchExpressions: []current.ImageSelectorRequirement{
			{Key: "url", Operator: "StartsWith", Values: []string{"docker.io/library/"}},
		}}}}}},
		"c8": {{ObjectMeta: metav1.ObjectMeta{Name: "c"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{ImageSelector: current.ImageSelector{MatchExpressions: []current.ImageSelectorRequirement{
			{Key: "url", Operator: "StartsWith", Values: []string{"docker.io/library/"}},
		}}}}}},
		"c4": {{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NameSelector: current.NameSelector{MatchExpressions: []current.NameSelectorRequirement{
			{Key: "anyContainer", Operator: "In", Values: []string{"c4", "cb"}},
		}}}}}},
		"cb": {{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Spec: current.InstrumentationSpec{ContainerSelector: current.ContainerSelector{NameSelector: current.NameSelector{MatchExpressions: []current.NameSelectorRequirement{
			{Key: "anyContainer", Operator: "In", Values: []string{"c4", "cb"}},
		}}}}}},
	}
	m := filterInstrumentations(ctx, insts, pod)
	if diff := cmp.Diff(expectedMap, m); diff != "" {
		t.Errorf("Unexpected diff (-want +got): %s", diff)
	}
}

func TestNewrelicConfigMapReplicator_ReplicateConfigMaps(t *testing.T) {
	configMapReplicator := NewNewrelicConfigMapReplicator(k8sClient)

	tests := []struct {
		name           string
		expectedErrStr string
		pod            corev1.Pod
		ns             corev1.Namespace
		operatorNs     string
		configMapName  string
		initConfigMaps []*corev1.ConfigMap
		initNs         []*corev1.Namespace
	}{
		{
			name:           "no configmap",
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "default",
			configMapName:  "newrelic-configmap",
			expectedErrStr: "configmaps \"newrelic-configmap\" not found",
		},
		{
			name: "same ns as operator",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns11"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns11"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns11"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns11",
			configMapName: "newrelic-configmap",
		},
		{
			name: "same ns as pod, no configmap in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns12-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns12-pod"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns12-pod"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns12-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns12-op",
			configMapName: "newrelic-configmap",
		},
		{
			name: "configmap in other ns, no configmaps in pod or operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns13-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns13-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns13-pod"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns13-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns13-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns13-op",
			configMapName:  "newrelic-configmap",
			expectedErrStr: "configmaps \"newrelic-configmap\" not found",
		},
		{
			name: "configmap in operator ns, no other configmaps",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns14-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns14-pod"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns14-op"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns14-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns14-op",
			configMapName: "newrelic-configmap",
		},
		{
			name: "operator ns, none in pod ns, configmap in other ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns15-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns15-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns15-other"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns15-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns15-other"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns15-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns15-op",
			configMapName: "newrelic-configmap",
		},
		{
			name: "configmap in operator ns, ns-specific configmap in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns16-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns16-pod"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns16-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod-configmap", Namespace: "ns16-pod"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns16-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns16-op",
			configMapName: "newrelic-configmap",
		},
		{
			name: "configmap in operator ns, ns-specific configmap in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns17-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns17-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns17-other"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns17-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns17-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns17-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns17-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns17-op",
			configMapName:  "not-a-normal-configmap",
			expectedErrStr: "configmaps \"not-a-normal-configmap\" not found",
		},
		{
			name: "configmap in operator ns, ns-specific configmap in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns18-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns18-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns18-other"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns18-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns18-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns18-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-configmap", Namespace: "ns18-other"}},
			},
			ns:             corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns18-pod"}},
			pod:            corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:     "ns18-op",
			configMapName:  "not-a-normal-configmap",
			expectedErrStr: "configmaps \"not-a-normal-configmap\" not found",
		},
		{
			name: "configmap in operator ns, ns-specific configmap in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns19-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns19-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns19-other"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns19-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns19-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns19-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-configmap", Namespace: "ns19-pod"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns19-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns19-op",
			configMapName: "not-a-normal-configmap",
		},
		{
			name: "configmap in operator ns, ns-specific configmap in pod ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "ns20-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns20-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "ns20-other"}},
			},
			initConfigMaps: []*corev1.ConfigMap{
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns20-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns20-pod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "newrelic-configmap", Namespace: "ns20-other"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "not-a-normal-configmap", Namespace: "ns20-op"}},
			},
			ns:            corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns20-pod"}},
			pod:           corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod"}},
			operatorNs:    "ns20-op",
			configMapName: "not-a-normal-configmap",
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%d_%s", i, tc.name), func(t *testing.T) {
			ctx := context.Background()

			for _, initNs := range tc.initNs {
				if initNs.Name == "default" || initNs.Name == "" {
					continue
				}
				err := k8sClient.Create(ctx, initNs)
				require.NoError(t, err)
			}
			defer func() {
				for _, initNs := range tc.initNs {
					if initNs.Name == "default" || initNs.Name == "" {
						continue
					}
					err := k8sClient.Delete(ctx, initNs)
					require.NoError(t, err)
				}
			}()

			for _, initConfigMap := range tc.initConfigMaps {
				err := k8sClient.Create(ctx, initConfigMap)
				require.NoError(t, err)
			}
			defer func() {
				for _, initConfigMap := range tc.initConfigMaps {
					err := k8sClient.Delete(ctx, initConfigMap)
					require.NoError(t, err)
				}
			}()

			err := configMapReplicator.ReplicateConfigMaps(ctx, tc.ns, tc.pod, tc.operatorNs, []string{tc.configMapName})
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if tc.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", tc.expectedErrStr, errStr)
			}
		})
	}
}

func TestValidateInstrumentations(t *testing.T) {
	tests := []struct {
		name          string
		insts         []*current.Instrumentation
		expectedErrIs error
		enabled       bool
	}{
		{
			name: "php hash dup",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.3", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-8.1", Image: "b"}}},
			},
			expectedErrIs: errMultipleInstancesPossible,
		},
		{
			name: "no dup",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo3"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo4"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo5"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-7.4", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo6"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "nodejs", Image: "a"}}},
			},
			expectedErrIs: nil,
		},
		{
			name: "java has dup",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo3"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "dotnet", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo4"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "python", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo5"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "php-7.4", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo6"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "nodejs", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo7"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "b"}}},
			},
			expectedErrIs: errMultipleInstancesPossible,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInstrumentations(tt.insts)
			if tt.expectedErrIs != nil && !errors.Is(err, tt.expectedErrIs) {
				t.Errorf("unexpected error string. expected: %s, got: %s", tt.expectedErrIs, err)
			}
		})
	}
}

func TestValidateContainerInstrumentations(t *testing.T) {
	boolAsPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name           string
		insts          []*current.Instrumentation
		expectedErrStr string
		enabled        bool
	}{
		{
			name: "agentConfigMap and env.name == NEWRELIC_FILE using in same instrumentation; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{AgentConfigMap: "yes", Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "NEWRELIC_FILE"}}}}},
			},
			expectedErrStr: "agentConfigMap and env.name == NEWRELIC_FILE can't be used together; conflicting instrumentations: [foo]",
		},
		{
			name: "agentConfigMap and env.name == NEWRELIC_FILE using in different instrumentations; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{AgentConfigMap: "yes", Agent: current.Agent{Language: "java", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "NEWRELIC_FILE"}}}}},
			},
			expectedErrStr: "agentConfigMap and env.name == NEWRELIC_FILE can't be used together; conflicting instrumentations: [foo2]\ninstrumentation.spec.agentConfigMap conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "multiple agentConfigMaps in the instrumentations; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{AgentConfigMap: "yes", Agent: current.Agent{Language: "java", Image: "a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{AgentConfigMap: "yes2", Agent: current.Agent{Language: "java", Image: "a"}}},
			},
			expectedErrStr: "instrumentation.spec.agentConfigMap conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "multiple instrumentations with env.NEWRELIC_FILE configured; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "NEWRELIC_FILE", Value: "a"}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "NEWRELIC_FILE", Value: "b"}}}}},
			},
			expectedErrStr: "instrumentation.spec.agent.env with entry keys [NEWRELIC_FILE], conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},

		{
			name: "multiple resources defined(one empty) in the instrumentation; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("23Mi"),
					},
				}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a"}}},
			},
			expectedErrStr: "instrumentation.spec.agent.resourceRequirements conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "multiple resources defined(both configured diff) in the instrumentation; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("23Mi"),
					},
				}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1.25"),
					},
				}}}},
			},
			expectedErrStr: "instrumentation.spec.agent.resourceRequirements conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "multiple security contexts defined(one empty) in the instrumentations; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", SecurityContext: &corev1.SecurityContext{
					Privileged: boolAsPtr(false),
				}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a"}}},
			},
			expectedErrStr: "instrumentation.spec.agent.securityContext conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "multiple security contexts defined(both configured diff) in the instrumentations; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem: boolAsPtr(true),
				}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a", SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot: boolAsPtr(true),
				}}}},
			},
			expectedErrStr: "instrumentation.spec.agent.securityContext conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "env vars with same name, diff values; not allowed",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "A"}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a", Env: []corev1.EnvVar{{Name: "A", Value: "B"}}}}},
			},
			expectedErrStr: "instrumentation.spec.agent.env with entry keys [A], conflicts with multiple instrumentations matching the same container; instrumentations: foo, foo2",
		},
		{
			name: "env vars with diff names; allowed; it should merge",
			insts: []*current.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "foo"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "java", Image: "a", Env: []corev1.EnvVar{{Name: "A"}}}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foo2"}, Spec: current.InstrumentationSpec{Agent: current.Agent{Language: "ruby", Image: "a", Env: []corev1.EnvVar{{Name: "B"}}}}},
			},
			expectedErrStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerInstrumentations(tt.insts)
			var errStr string
			if err != nil {
				errStr = err.Error()
			}
			if tt.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", tt.expectedErrStr, errStr)
			}
		})
	}
}

func TestValidateInstrumentationAgentConfigMaps(t *testing.T) {
	//validateInstrumentationAgentConfigMaps()
}

func TestValidateHealthAgents(t *testing.T) {
	//validateHealthAgents()
}

func TestValidateContainerWithHealthAgent(t *testing.T) {
	//validateContainerWithHealthAgent()
}

func TestValidateAgentConfigMap(t *testing.T) {
	//validateAgentConfigMap()
}
