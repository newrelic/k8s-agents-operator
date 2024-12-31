package webhook

import (
	"context"
	"encoding/json"
	"github.com/go-logr/logr"
	"github.com/newrelic/k8s-agents-operator/src/internal/apm"
	"github.com/newrelic/k8s-agents-operator/src/internal/instrumentation"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// compile time type assertion
var (
	_ PodMutator = (*instrumentation.InstrumentationPodMutator)(nil)
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,sideEffects=none,admissionReviewVersions=v1
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups=newrelic.com,resources=instrumentations,verbs=get;list;watch
// +kubebuilder:rbac:groups="apps",resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;create;delete;deletecollection;patch;update;watch

// PodMutationHandler is a webhook handler for mutating Pods
type PodMutationHandler struct {
	Client   client.Client
	Decoder  admission.Decoder
	Mutators []PodMutator
}

// PodMutator mutates a pod.
type PodMutator interface {
	Mutate(ctx context.Context, ns corev1.Namespace, pod corev1.Pod) (corev1.Pod, error)
}

var podMutatorLog = ctrl.Log.WithName("pod-mutator")

// Handle manages Pod mutations
func (m *PodMutationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := corev1.Pod{}
	err := m.Decoder.Decode(req, &pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	podMutatorLog.Info("Mutating Pod", "name", pod.Name)

	// we use the req.Namespace here because the pod might have not been created yet
	ns := corev1.Namespace{}
	err = m.Client.Get(ctx, types.NamespacedName{Name: req.Namespace, Namespace: ""}, &ns)
	if err != nil {
		res := admission.Errored(http.StatusInternalServerError, err)
		// By default, admission.Errored sets Allowed to false which blocks pod creation even though the failurePolicy=ignore.
		// Allowed set to true makes sure failure does not block pod creation in case of an error.
		// Using the http.StatusInternalServerError creates a k8s event associated with the replica set.
		// The admission.Allowed("").WithWarnings(err.Error()) or http.StatusBadRequest does not
		// create any event. Additionally, an event/log cannot be created explicitly because the pod name is not known.
		res.Allowed = true
		return res
	}

	for _, mutator := range m.Mutators {
		pod, err = mutator.Mutate(ctx, ns, pod)
		if err != nil {
			//@todo: actually print the error message
			res := admission.Errored(http.StatusInternalServerError, err)
			res.Allowed = true
			return res
		}
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		podMutatorLog.Error(err, "failed to marshal pod")
		res := admission.Errored(http.StatusInternalServerError, err)
		res.Allowed = true
		return res
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// SetupWebhookWithManager registers the pod mutation webhook
func SetupWebhookWithManager(mgr ctrl.Manager, operatorNamespace string, logger logr.Logger) error {
	// Setup InstrumentationMutator
	mgrClient := mgr.GetClient()
	injectorRegistry := apm.DefaultInjectorRegistry
	injector := instrumentation.NewNewrelicSdkInjector(logger, mgrClient, injectorRegistry)
	secretReplicator := instrumentation.NewNewrelicSecretReplicator(logger, mgrClient)
	instrumentationLocator := instrumentation.NewNewRelicInstrumentationLocator(logger, mgrClient, operatorNamespace)

	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-v1-pod", &webhook.Admission{Handler: &PodMutationHandler{
		Client:  mgr.GetClient(),
		Decoder: admission.NewDecoder(mgr.GetScheme()),
		Mutators: []PodMutator{
			instrumentation.NewMutator(
				logger,
				mgrClient,
				injector,
				secretReplicator,
				instrumentationLocator,
				operatorNamespace,
			),
		},
	}})

	return nil
}
