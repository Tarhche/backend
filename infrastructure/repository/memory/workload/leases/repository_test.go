package leases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/leasestest"
)

func TestRepository(t *testing.T) {
	t.Parallel()

	leasestest.Repository(t, func(*testing.T) graph.Leases {
		return NewRepository(nil)
	})
}

func TestRepository_clock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a lease expires, and a backoff ends, as the clock it is given says, and only then", func(t *testing.T) {
		t.Parallel()

		clock := &manual{now: moment}
		repository := NewRepository(clock.Now)

		_, err := repository.Take(ctx, "root", "one", 30*time.Second)
		require.NoError(t, err)

		clock.Advance(29 * time.Second)

		_, err = repository.Take(ctx, "root", "another", time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld, "a second before it expires")

		clock.Advance(time.Second)

		taken, err := repository.Take(ctx, "root", "another", time.Minute)
		require.NoError(t, err, "and taken once it has")
		assert.Equal(t, moment.Add(30*time.Second), taken.UpdatedAt)

		released, err := repository.Release(ctx, "root", "another", graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: graph.Backoff{First: 15 * time.Second, Most: 15 * time.Minute}})
		require.NoError(t, err)
		assert.Equal(t, moment.Add(45*time.Second), released.NextAt)

		clock.Advance(14 * time.Second)

		_, err = repository.Take(ctx, "root", "one", time.Minute)
		require.ErrorIs(t, err, graph.ErrBackedOff)

		clock.Advance(time.Second)

		_, err = repository.Take(ctx, "root", "one", time.Minute)
		require.NoError(t, err)

		kept, ok := repository.Lease("root")
		require.True(t, ok)
		assert.Equal(t, "one", kept.Holder)
		assert.Equal(t, 1, kept.Failures)
	})

	t.Run("what it is told to fail with is what every call fails with", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		repository := NewRepository(nil)
		repository.Fail = broken

		_, err := repository.Take(ctx, "root", "one", time.Minute)
		assert.ErrorIs(t, err, broken)

		_, err = repository.Renew(ctx, "root", "one", time.Minute)
		assert.ErrorIs(t, err, broken)

		_, err = repository.Release(ctx, "root", "one", graph.Ended{State: graph.StateDone})
		assert.ErrorIs(t, err, broken)

		assert.ErrorIs(t, repository.Forget(ctx, "root"), broken)

		_, err = repository.Get(ctx, "root")
		assert.ErrorIs(t, err, broken)

		_, kept := repository.Lease("root")
		assert.False(t, kept)
	})
}

// moment is when a test happens, as a clock says it.
var moment = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// manual is a clock a test moves on by hand.
type manual struct {
	lock sync.Mutex
	now  time.Time
}

func (c *manual) Now() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.now
}

func (c *manual) Advance(d time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.now = c.now.Add(d)
}
