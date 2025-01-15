package instrumentation

import (
	"context"

	"github.com/newrelic/k8s-agents-operator/api/v1beta1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InstrumentationStatusUpdater interface {
	UpdateInstrumentationStatus(ctx context.Context, instrumentation *v1beta1.Instrumentation) error
}

type InstrumentationStatusUpdaterImpl struct {
	client.Client
}

func NewInstrumentationStatusUpdater(client client.Client) *InstrumentationStatusUpdaterImpl {
	return &InstrumentationStatusUpdaterImpl{Client: client}
}

func (i *InstrumentationStatusUpdaterImpl) UpdateInstrumentationStatus(ctx context.Context, instrumentation *v1beta1.Instrumentation) error {
	if err := i.Client.Status().Update(ctx, instrumentation); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
