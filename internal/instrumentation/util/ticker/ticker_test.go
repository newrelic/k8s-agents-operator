package ticker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTicker(t *testing.T) {
	c := 0
	tk := NewTicker(time.Millisecond*15, func(ctx context.Context, ts time.Time) {
		c++
	})
	time.Sleep(time.Millisecond * 20)
	tk.Shutdown(context.Background())
	assert.Equal(t, c, 1)
	tk.Stop(context.Background())
}
