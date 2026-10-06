package lock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocks(t *testing.T) {
	t.Parallel()

	t.Run("what is done to one VM is done one thing at a time", func(t *testing.T) {
		t.Parallel()

		locks := New()

		var inside, most atomic.Int32
		var wg sync.WaitGroup

		for range 20 {
			wg.Go(func() {
				release, err := locks.Lock(t.Context(), "vm-1")
				require.NoError(t, err)
				defer release()

				now := inside.Add(1)
				for {
					seen := most.Load()
					if now <= seen || most.CompareAndSwap(seen, now) {
						break
					}
				}

				time.Sleep(time.Millisecond)
				inside.Add(-1)
			})
		}

		wg.Wait()

		assert.Equal(t, int32(1), most.Load())
	})

	t.Run("different VMs do not wait for each other", func(t *testing.T) {
		t.Parallel()

		locks := New()

		release, err := locks.Lock(t.Context(), "vm-1")
		require.NoError(t, err)
		defer release()

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		other, err := locks.Lock(ctx, "vm-2")
		require.NoError(t, err)
		other()
	})

	t.Run("giving up on waiting holds nothing", func(t *testing.T) {
		t.Parallel()

		locks := New()

		release, err := locks.Lock(t.Context(), "vm-1")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()

		_, err = locks.Lock(ctx, "vm-1")
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		release()
		release()

		again, err := locks.Lock(t.Context(), "vm-1")
		require.NoError(t, err, "released twice is released once")
		again()
	})
}
