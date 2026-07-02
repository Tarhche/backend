package profiler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResourceGuardAdmit(t *testing.T) {
	t.Run("limits profiles per minute", func(t *testing.T) {
		g := newResourceGuard(2, 1<<20)

		assert.NoError(t, g.admit())
		assert.NoError(t, g.admit())
		assert.ErrorIs(t, g.admit(), errRateLimited)
	})

	t.Run("resets the budget once the window passed", func(t *testing.T) {
		g := newResourceGuard(1, 1<<20)

		now := time.Now()
		g.now = func() time.Time { return now }

		assert.NoError(t, g.admit())
		assert.ErrorIs(t, g.admit(), errRateLimited)

		g.now = func() time.Time { return now.Add(time.Minute) }
		assert.NoError(t, g.admit())
	})
}

func TestResourceGuardReserve(t *testing.T) {
	t.Run("caps buffered payload bytes", func(t *testing.T) {
		g := newResourceGuard(10, 100)

		assert.NoError(t, g.reserve(60))
		assert.ErrorIs(t, g.reserve(60), errBufferFull)
		assert.Equal(t, int64(60), g.bufferedBytes())
	})

	t.Run("release frees the budget", func(t *testing.T) {
		g := newResourceGuard(10, 100)

		assert.NoError(t, g.reserve(60))
		g.release(60)
		assert.NoError(t, g.reserve(100))
		assert.Equal(t, int64(100), g.bufferedBytes())
	})
}
