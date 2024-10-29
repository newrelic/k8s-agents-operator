package instrumentation

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/api/v1alpha2"
	"github.com/newrelic-experimental/k8s-agents-operator-windows/src/apm"
)

type FakeInjector func(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod

func (fn FakeInjector) Inject(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	return fn(ctx, insts, ns, pod)
}

type FakeSecretReplicator func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error

func (fn FakeSecretReplicator) ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error {
	return fn(ctx, ns, pod, operatorNamespace, secretName)
}

type FakeInstrumentationLocator func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error)

func (fn FakeInstrumentationLocator) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error) {
	return fn(ctx, ns, pod)
}

type InstrumentationLocatorFn func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error)

func (il InstrumentationLocatorFn) GetInstrumentations(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error) {
	return il(ctx, ns, pod)
}

var _ InstrumentationLocator = (InstrumentationLocatorFn)(nil)

type SdkInjectorFn func(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod

func (si SdkInjectorFn) Inject(ctx context.Context, insts []*v1alpha2.Instrumentation, ns corev1.Namespace, pod corev1.Pod) corev1.Pod {
	return si(ctx, insts, ns, pod)
}

var _ SdkInjector = (SdkInjectorFn)(nil)

type SecretReplicatorFn func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error

func (sr SecretReplicatorFn) ReplicateSecret(ctx context.Context, ns corev1.Namespace, pod corev1.Pod, operatorNamespace string, secretName string) error {
	return sr(ctx, ns, pod, operatorNamespace, secretName)
}

var _ SecretReplicator = (SecretReplicatorFn)(nil)

func TestMutatePod(t *testing.T) {
	var fakeInjector FakeInjector = func(
		ctx context.Context,
		insts []*v1alpha2.Instrumentation,
		ns corev1.Namespace,
		pod corev1.Pod,
	) corev1.Pod {
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		for _, inst := range insts {
			if inst.Spec.Agent.Language != "" {
				pod.Annotations["newrelic-"+inst.Spec.Agent.Language] = "true"
			}
		}
		return pod
	}
	var fakeSecretReplicator FakeSecretReplicator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
		operatorNamespace string,
		secretName string,
	) error {
		return nil
	}
	var _ = fakeSecretReplicator
	var fakeInstrumentationLocator FakeInstrumentationLocator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
	) ([]*v1alpha2.Instrumentation, error) {
		return nil, nil
	}
	var fakeInstrumentationLocatorWithDup FakeInstrumentationLocator = func(
		ctx context.Context,
		ns corev1.Namespace,
		pod corev1.Pod,
	) ([]*v1alpha2.Instrumentation, error) {
		return []*v1alpha2.Instrumentation{
			{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "ruby", Image: "ruby1"}}},
			{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "ruby", Image: "ruby2"}}},
		}, nil
	}
	logger := logr.Discard()

	tests := []struct {
		name        string
		pod         corev1.Pod
		ns          corev1.Namespace
		initInsts   []*v1alpha2.Instrumentation
		initNs      []*corev1.Namespace
		initSecrets []*corev1.Secret
		operatorNs  string

		expectedPod     corev1.Pod
		expectedSecrets []client.ObjectKey
		expectedErrStr  string

		injector               SdkInjector
		instrumentationLocator InstrumentationLocator
		secretReplicator       SecretReplicator
	}{
		{
			name: "java injection, true",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns1-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns1-pod"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns1-op"},
					Spec:       v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}},
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
			instrumentationLocator: InstrumentationLocatorFn(func(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) ([]*v1alpha2.Instrumentation, error) {
				return nil, fmt.Errorf("fetch instrumentation error")
			}),
			pod:            corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:    corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedErrStr: "fetch instrumentation error",
		},
		{
			name:                   "no instrumentations",
			instrumentationLocator: fakeInstrumentationLocator,
			pod:                    corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:            corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedErrStr:         errNoInstancesAvailable.Error(),
		},
		{
			name:                   "some error when getting language instrumentations",
			instrumentationLocator: fakeInstrumentationLocatorWithDup,
			pod:                    corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod:            corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns4-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns4-pod"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns4-op"},
					Spec:       v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}},
				},
			},
			operatorNs:     "gns4-op",
			expectedErrStr: errMultipleInstancesPossible.Error(),
		},
		{
			name:        "conflicting secret names",
			ns:          corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gns5-pod"}},
			pod:         corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			injector:    fakeInjector,
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns5-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns5-pod"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns5-op"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: "", Agent: v1alpha2.Agent{Language: "java", Image: "java"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-php", Namespace: "gns5-op"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: "different", Agent: v1alpha2.Agent{Language: "php", Image: "php"}},
				},
			},
			initSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{Name: DefaultLicenseKeySecretName, Namespace: "gns5-op"},
					Data:       map[string][]byte{apm.LicenseKey: []byte(base64.RawStdEncoding.EncodeToString([]byte("abc123")))},
				},
			},
			operatorNs: "gns5-op",
		},
		{
			name:        "secret doesn't exist",
			ns:          corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gns6-pod"}},
			pod:         corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			expectedPod: corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
			injector:    fakeInjector,
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "gns6-op"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "gns6-pod"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-java", Namespace: "gns6-op"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: "", Agent: v1alpha2.Agent{Language: "java", Image: "java"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "example-inst-php", Namespace: "gns6-op"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: "", Agent: v1alpha2.Agent{Language: "php", Image: "php"}},
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
				apmInjectors := []apm.Injector{
					&apm.DotnetInjector{},
					&apm.DotnetWindowsInjector{},
					&apm.GoInjector{},
					&apm.JavaInjector{},
					&apm.NodejsInjector{},
					&apm.PhpInjector{},
					&apm.PythonInjector{},
					&apm.RubyInjector{},
				}
				for _, apmInjector := range apmInjectors {
					injectorRegistry.MustRegister(apmInjector)
				}
				injector = NewNewrelicSdkInjector(logger, k8sClient, injectorRegistry)
			}
			instrumentationLocator := test.instrumentationLocator
			if instrumentationLocator == nil {
				instrumentationLocator = NewNewRelicInstrumentationLocator(logger, k8sClient, test.operatorNs)
			}
			secretReplicator := test.secretReplicator
			if secretReplicator == nil {
				secretReplicator = NewNewrelicSecretReplicator(logger, k8sClient)
			}

			mutator := NewMutator(
				logger,
				k8sClient,
				injector,
				secretReplicator,
				instrumentationLocator,
				test.operatorNs,
			)
			resultPod, err := mutator.Mutate(ctx, test.ns, test.pod)
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
			}
		})
	}
}

func TestNewrelicSecretReplicator_ReplicateSecret(t *testing.T) {
	logger := logr.Discard()
	secretReplicator := NewNewrelicSecretReplicator(logger, k8sClient)

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

			err := secretReplicator.ReplicateSecret(ctx, tc.ns, tc.pod, tc.operatorNs, tc.secretName)
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
		instrumentations  []*v1alpha2.Instrumentation
		expectedLangInsts []*v1alpha2.Instrumentation
		expectedErrStr    string
	}{
		{
			name: "none",
		},
		{
			name: "dotnet",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "dotnet", Image: "dotnet"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "dotnet", Image: "dotnet"}}},
			},
		},
		{
			name: "go",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "go", Image: "go"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "go", Image: "go"}}},
			},
		},
		{
			name: "java",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
		},
		{
			name: "nodejs",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "nodejs", Image: "nodejs"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "nodejs", Image: "nodejs"}}},
			},
		},
		{
			name: "php",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php", Image: "php"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php", Image: "php"}}},
			},
		},
		{
			name: "python",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "python", Image: "python"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "python", Image: "python"}}},
			},
		},
		{
			name: "ruby",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "ruby", Image: "ruby"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "ruby", Image: "ruby"}}},
			},
		},
		{
			name: "java + php",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php", Image: "php"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "php", Image: "php"}}},
			},
		},
		{
			name: "java + java, both identical, only return first occurrence",
			instrumentations: []*v1alpha2.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "1st"}, Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "2st"}, Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
			expectedLangInsts: []*v1alpha2.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "1st"}, Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
		},
		{
			name: "java + java, env value different",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "1"}}}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "0"}}}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select",
		},
		{
			name: "java + java, env name different",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "DEBUG", Value: "1"}}}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", Env: []corev1.EnvVar{{Name: "LOGGING", Value: "1"}}}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select",
		},
		{
			name: "java + java, image different",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java-is-great"}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java-is-terrible"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select",
		},
		{
			name: "java + java, volume size different",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", VolumeSizeLimit: resource.NewQuantity(2, resource.DecimalSI)}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select",
		},
		{
			name: "java + java, resources different",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}}}}},
				{Spec: v1alpha2.InstrumentationSpec{Agent: v1alpha2.Agent{Language: "java", Image: "java"}}},
			},
			expectedErrStr: "multiple New Relic Instrumentation instances available, cannot determine which one to select",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			langInsts, err := GetLanguageInstrumentations(test.instrumentations)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if test.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", test.expectedErrStr, errStr)
			}
			if diff := cmp.Diff(test.expectedLangInsts, langInsts); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestGetSecretNameFromInstrumentations(t *testing.T) {
	tests := []struct {
		name               string
		instrumentations   []*v1alpha2.Instrumentation
		expectedSecretName string
		expectedErrStr     string
	}{
		{
			name: "none",
		},
		{
			name:               "one, default",
			instrumentations:   []*v1alpha2.Instrumentation{{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}}},
			expectedSecretName: DefaultLicenseKeySecretName,
		},
		{
			name:               "one, blank",
			instrumentations:   []*v1alpha2.Instrumentation{{Spec: v1alpha2.InstrumentationSpec{}}},
			expectedSecretName: DefaultLicenseKeySecretName,
		},
		{
			name:               "one, other",
			instrumentations:   []*v1alpha2.Instrumentation{{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}}},
			expectedSecretName: "something-else",
		},
		{
			name: "two, one blank, the other the default",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
			},
			expectedSecretName: DefaultLicenseKeySecretName,
		},
		{
			name: "three, one something else, one the default, one blank",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
				{Spec: v1alpha2.InstrumentationSpec{}},
			},
			expectedErrStr: "multiple key secrets",
		},
		{
			name: "two, one blank, the other the something else",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
			expectedErrStr: "multiple key secrets",
		},
		{
			name: "two, one the default, the other the something else",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
			expectedErrStr: "multiple key secrets",
		},
		{
			name: "two, one blank, the other the default",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName}},
			},
			expectedSecretName: DefaultLicenseKeySecretName,
		},
		{
			name: "two, both something else",
			instrumentations: []*v1alpha2.Instrumentation{
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}},
				{Spec: v1alpha2.InstrumentationSpec{LicenseKeySecret: "something-else"}},
			},
			expectedSecretName: "something-else",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secretName, err := GetSecretNameFromInstrumentations(test.instrumentations)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			if test.expectedErrStr != errStr {
				t.Errorf("unexpected error string. expected: %s, got: %s", test.expectedErrStr, errStr)
			}
			if test.expectedSecretName != secretName {
				t.Errorf("unexpected secret name. expected: %s, got: %s", test.expectedSecretName, errStr)
			}
		})
	}
}

func TestNewrelicInstrumentationLocator_GetInstrumentations(t *testing.T) {
	logger := logr.Discard()
	tests := []struct {
		name           string
		expectedErrStr string
		initNs         []*corev1.Namespace
		initInsts      []*v1alpha2.Instrumentation
		ns             corev1.Namespace
		pod            corev1.Pod
		operatorNs     string
		insts          []*v1alpha2.Instrumentation
	}{
		{
			name: "none",
		},
		{
			name: "not in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "other1"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst2", Namespace: "operator2"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst3", Namespace: "operator3"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "inst4-1", Namespace: "operator4-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "inst4-2", Namespace: "operator4-2"}},
			},
			operatorNs: "operator4-1",
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod4"}},
			insts: []*v1alpha2.Instrumentation{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst4-1", Namespace: "operator4-1"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
			},
		},
		{
			name: "2 in operator ns",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator5"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{ObjectMeta: metav1.ObjectMeta{Name: "inst5-1", Namespace: "operator5"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "inst5-2", Namespace: "operator5"}},
			},
			operatorNs: "operator5",
			pod:        corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod5"}},
			insts: []*v1alpha2.Instrumentation{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst5-1", Namespace: "operator5"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
				{
					TypeMeta:   metav1.TypeMeta{Kind: "Instrumentation"},
					ObjectMeta: metav1.ObjectMeta{Name: "inst5-2", Namespace: "operator5"},
					Spec:       v1alpha2.InstrumentationSpec{LicenseKeySecret: DefaultLicenseKeySecretName},
				},
			},
		},
		{
			name: "1 matching with pod selector",
			initNs: []*corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "operator6"}},
			},
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst6", Namespace: "operator6"},
					Spec: v1alpha2.InstrumentationSpec{
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
			insts: []*v1alpha2.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst6", Namespace: "operator6"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst7", Namespace: "operator7"},
					Spec: v1alpha2.InstrumentationSpec{
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
			insts: []*v1alpha2.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst7", Namespace: "operator7"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst8", Namespace: "operator8"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst9", Namespace: "operator9"},
					Spec: v1alpha2.InstrumentationSpec{
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
			initInsts: []*v1alpha2.Instrumentation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "inst10", Namespace: "operator10"},
					Spec: v1alpha2.InstrumentationSpec{
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
			insts: []*v1alpha2.Instrumentation{
				{
					TypeMeta: metav1.TypeMeta{Kind: "Instrumentation"}, ObjectMeta: metav1.ObjectMeta{Name: "inst10", Namespace: "operator10"},
					Spec: v1alpha2.InstrumentationSpec{
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
	instSorter := func(a, b *v1alpha2.Instrumentation) bool {
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

			locator := NewNewRelicInstrumentationLocator(logger, k8sClient, test.operatorNs)
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
