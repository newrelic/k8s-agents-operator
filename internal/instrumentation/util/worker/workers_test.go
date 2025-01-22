package worker

import (
	"context"
	"testing"
)

func TestGetWorkerID(t *testing.T) {
	if GetWorkerID(context.WithValue(context.Background(), ctxWorkerID, int64(123))) != int64(123) {
		t.Error("worker id should be 123")
	}
}
