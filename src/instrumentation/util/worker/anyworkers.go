package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type AnyWorkers struct {
	pending int64
	/// added for cpu cache alignment to 32 bytes
	_padding1 [3]int64

	lastWorkerID int64
	/// added for cpu cache alignment to 32 bytes
	_padding2 [3]int64

	handler       WorkerHandler
	stopCh        chan struct{}
	stopOnce      *sync.Once
	stopCtx       context.Context
	stopCtxCancel context.CancelCauseFunc
	shutdownOnce  *sync.Once
	shutdownCh    chan struct{}
	doneCh        chan struct{}
	wg            *sync.WaitGroup
}

func NewAnyWorkers(handler WorkerHandler) *AnyWorkers {
	stopCtx, stopCtxCancel := context.WithCancelCause(context.Background())
	w := &AnyWorkers{
		doneCh:        make(chan struct{}),
		shutdownCh:    make(chan struct{}),
		stopCh:        make(chan struct{}),
		shutdownOnce:  &sync.Once{},
		stopOnce:      &sync.Once{},
		handler:       handler,
		wg:            &sync.WaitGroup{},
		stopCtx:       stopCtx,
		stopCtxCancel: stopCtxCancel,
	}
	return w
}

func (w *AnyWorkers) Shutdown(ctx context.Context) error {
	w.shutdown()
	return w.waitForDone(ctx)
}

func (w *AnyWorkers) Stop(ctx context.Context) error {
	w.shutdown()
	w.stop()
	return w.waitForDone(ctx)
}

func (w *AnyWorkers) stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.stopCtxCancel(ErrStopped)
	})
}

func (w *AnyWorkers) shutdown() {
	w.stopOnce.Do(func() {
		close(w.shutdownCh)
		w.waitForPending()
		go func() {
			w.wg.Wait()
			close(w.doneCh)
			w.stop()
		}()
	})
}

func (w *AnyWorkers) waitForPending() {
	for atomic.LoadInt64(&w.pending) > 0 {
		time.Sleep(time.Microsecond * 50)
	}
}

func (w *AnyWorkers) waitForDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.doneCh:
		return nil
	}
}

func (w *AnyWorkers) Add(ctx context.Context, data any) error {
	select {
	case <-w.shutdownCh:
		return ErrShutdown
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	atomic.AddInt64(&w.pending, 1)
	defer atomic.AddInt64(&w.pending, -1)
	select {
	case <-w.shutdownCh:
		return ErrShutdown
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	w.wg.Add(1)
	go w.newWorker(atomic.AddInt64(&w.lastWorkerID, 1), data)
	return nil
}

func (w *AnyWorkers) newWorker(workerID int64, data any) {
	defer w.wg.Done()
	stopCtx := context.WithValue(w.stopCtx, ctxWorkerID, workerID)
	w.handler(stopCtx, data)
}
