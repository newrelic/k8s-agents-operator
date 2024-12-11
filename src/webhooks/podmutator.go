package webhooks

import (
	"context"
	"encoding/json"
	"github.com/newrelic/k8s-agents-operator/src/apm"
	"github.com/newrelic/k8s-agents-operator/src/internal/instrumentation"
	"github.com/newrelic/k8s-agents-operator/src/internal/webhookhandler"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// PodMutator is a webhook handler for mutating Pods
type PodMutator struct {
	Client  client.Client
	Decoder *admission.Decoder
}

var podMutatorLog = ctrl.Log.WithName("pod-mutator")
var instrumentationMutators []webhookhandler.PodMutator

// InjectDecoder injects the decoder
func (m *PodMutator) InjectDecoder(d *admission.Decoder) error {
	m.Decoder = d
	return nil
}

// Handle manages Pod mutations
func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := corev1.Pod{}
	err := (*m.Decoder).Decode(req, &pod)
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

	for _, im := range instrumentationMutators {
		pod, err = im.Mutate(ctx, ns, pod)
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
func SetupWebhookWithManager(mgr ctrl.Manager, operatorNamespace string) error {
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-v1-pod", &webhook.Admission{Handler: &PodMutator{
		Client: mgr.GetClient(),
	}})

	// Setup InstrumentationMutator
	mgrClient := mgr.GetClient()
	injectorRegistry := apm.DefaultInjectorRegistry
	injector := instrumentation.NewNewrelicSdkInjector(mgrClient, injectorRegistry)
	secretReplicator := instrumentation.NewNewrelicSecretReplicator(mgrClient)
	instrumentationLocator := instrumentation.NewNewRelicInstrumentationLocator(mgrClient, operatorNamespace)
	instrumentationMutators = []webhookhandler.PodMutator{
		instrumentation.NewMutator(
			mgrClient,
			injector,
			secretReplicator,
			instrumentationLocator,
			operatorNamespace,
		),
	}

	return nil
}
