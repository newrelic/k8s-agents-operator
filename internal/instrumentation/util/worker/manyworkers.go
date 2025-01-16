package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type ManyWorkers struct {
	pending int64
	/// added for cpu cache alignment to 32 bytes
	_padding1 [3]int64

	lastWorkerID int64
	/// added for cpu cache alignment to 32 bytes
	_padding2 [3]int64

	queueCh       chan any
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

func NewManyWorkers(count int, size int, handler WorkerHandler) *ManyWorkers {
	w := &ManyWorkers{
		queueCh:      make(chan any, size),
		doneCh:       make(chan struct{}),
		shutdownCh:   make(chan struct{}),
		stopCh:       make(chan struct{}),
		shutdownOnce: &sync.Once{},
		stopOnce:     &sync.Once{},
		handler:      handler,
		wg:           &sync.WaitGroup{},
	}
	w.stopCtx, w.stopCtxCancel = context.WithCancelCause(context.Background())
	for i := 0; i < count; i++ {
		w.wg.Add(1)
		go w.newWorker(atomic.AddInt64(&w.lastWorkerID, 1))
	}
	return w
}

func (w *ManyWorkers) Shutdown(ctx context.Context) error {
	w.shutdown()
	return w.waitForDone(ctx)
}

func (w *ManyWorkers) Stop(ctx context.Context) error {
	w.shutdown()
	w.stop()
	return w.waitForDone(ctx)
}

func (w *ManyWorkers) stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.stopCtxCancel(ErrStopped)
	})
}

func (w *ManyWorkers) shutdown() {
	w.stopOnce.Do(func() {
		close(w.shutdownCh)
		w.waitForPending()
		close(w.queueCh)
		go func() {
			w.wg.Wait()
			close(w.doneCh)
			w.stop()
		}()
	})
}

func (w *ManyWorkers) waitForPending() {
	for atomic.LoadInt64(&w.pending) > 0 {
		time.Sleep(time.Microsecond * 50)
	}
}

func (w *ManyWorkers) waitForDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.doneCh:
		return nil
	}
}

func (w *ManyWorkers) Add(ctx context.Context, data any) error {
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
	select {
	case w.queueCh <- data:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-w.shutdownCh:
		return ErrShutdown
	}
}

func (w *ManyWorkers) newWorker(workerID int64) {
	defer w.wg.Done()
	stopCtx := context.WithValue(w.stopCtx, ctxWorkerID, workerID)
	for {
		select {
		case <-w.stopCh:
			return
		case data, more := <-w.queueCh:
			if !more {
				return
			}
			w.handler(stopCtx, data)
		}
	}
}
