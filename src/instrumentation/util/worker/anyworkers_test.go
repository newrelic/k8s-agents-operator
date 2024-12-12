package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnyWorkers(t *testing.T) {
	var c int64
	measureCh := make(chan struct{})
	w := NewAnyWorkers(func(ctx context.Context, data any) {
		atomic.AddInt64(&c, 1)
		defer atomic.AddInt64(&c, -1)
		<-measureCh
	})
	for i := 0; i < 50; i++ {
		w.Add(context.Background(), nil)
	}
	time.Sleep(time.Millisecond)
	if c := atomic.LoadInt64(&c); c != 50 {
		t.Errorf("got %d, want 50", c)
	}
	close(measureCh)
	_ = w.Shutdown(context.Background())
	_ = w.Stop(context.Background())
	if c := atomic.LoadInt64(&c); c != 0 {
		t.Errorf("got %d, want 0", c)
	}
}
