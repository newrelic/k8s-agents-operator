package ticker

import (
	"context"
	"github.com/newrelic/k8s-agents-operator/src/instrumentation/util/worker"
	"sync"
	"time"
)

type TickHandler func(ctx context.Context, ts time.Time)

type Ticker struct {
	stopOnce      *sync.Once
	shutdownOnce  *sync.Once
	stopCtx       context.Context
	stopCtxCancel context.CancelCauseFunc
	stopCh        chan struct{}
	shutdownCh    chan struct{}
	doneCh        chan struct{}
	ticker        *time.Ticker
	handler       TickHandler
}

func NewTicker(interval time.Duration, handler TickHandler) *Ticker {
	ctx, cancel := context.WithCancelCause(context.Background())
	t := &Ticker{
		stopCh:        make(chan struct{}),
		shutdownCh:    make(chan struct{}),
		doneCh:        make(chan struct{}),
		shutdownOnce:  &sync.Once{},
		stopOnce:      &sync.Once{},
		ticker:        time.NewTicker(interval),
		handler:       handler,
		stopCtx:       ctx,
		stopCtxCancel: cancel,
	}
	go t.newTicker()
	return t
}

func (t *Ticker) waitForDone(ctx context.Context) error {
	select {
	case <-t.doneCh:
		return nil
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.doneCh:
		return nil
	}
}

func (t *Ticker) Stop(ctx context.Context) error {
	t.shutdown()
	t.stop()
	return t.waitForDone(ctx)
}

func (t *Ticker) Shutdown(ctx context.Context) error {
	t.shutdown()
	return t.waitForDone(ctx)
}

func (t *Ticker) stop() {
	t.stopOnce.Do(func() {
		close(t.stopCh)
		t.stopCtxCancel(worker.ErrStopped)
	})
}

func (t *Ticker) shutdown() {
	t.shutdownOnce.Do(func() {
		close(t.shutdownCh)
		t.ticker.Stop()
		for more := true; more; {
			select {
			case _, more = <-t.ticker.C:
			default:
				more = false
			}
		}
		go func() {
			<-t.doneCh
			t.stop()
		}()
	})
}

func (t *Ticker) newTicker() {
	defer close(t.doneCh)
	defer t.ticker.Stop()
	for {
		select {
		case <-t.stopCh:
			return
		case <-t.shutdownCh:
			return
		default:
		}
		select {
		case <-t.stopCh:
			return
		case <-t.shutdownCh:
			return
		case ts := <-t.ticker.C:
			t.handler(t.stopCtx, ts)
		}
	}
}
