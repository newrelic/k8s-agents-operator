package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestManyWorkers(t *testing.T) {
	var workersWaiting int64
	measureCh := make(chan struct{})
	w := NewManyWorkers(5, 5, func(ctx context.Context, data any) {
		atomic.AddInt64(&workersWaiting, 1)
		defer atomic.AddInt64(&workersWaiting, -1)
		<-measureCh
	})
	var jobsSent int64
	go func() {
		// add these in a go routine, because otherwise it will block until a worker can pick it up
		for i := 0; i < 50; i++ {
			w.Add(context.Background(), nil)
			atomic.AddInt64(&jobsSent, 1)
		}
	}()
	time.Sleep(time.Millisecond)
	if c := atomic.LoadInt64(&workersWaiting); c != 5 {
		t.Errorf("got %d, want 5", c)
	}
	if c := atomic.LoadInt64(&jobsSent); c != 10 {
		t.Errorf("got %d, want 10", c)
	}
	close(measureCh)
	_ = w.Shutdown(context.Background())
	_ = w.Stop(context.Background())
	if c := atomic.LoadInt64(&workersWaiting); c != 0 {
		t.Errorf("got %d, want 0", c)
	}
}
