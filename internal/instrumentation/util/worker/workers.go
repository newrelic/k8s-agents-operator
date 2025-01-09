package worker

import (
	"context"
	"errors"
)

type Worker interface {
	Shutdown(ctx context.Context) error
	Stop(ctx context.Context) error
	Add(ctx context.Context, data any) error
}

type WorkerHandler func(ctx context.Context, data any)

type ctxWorkerIDType struct{}

var (
	ErrShutdown = errors.New("shutdown")
	ErrStopped  = errors.New("stopped")
)

var ctxWorkerID ctxWorkerIDType

func GetWorkerID(ctx context.Context) int64 {
	if v := ctx.Value(ctxWorkerID); v != nil {
		return v.(int64)
	}
	return 0
}
