// Package leasestest holds a graph.Leases to what every one of them promises,
// so that the one kept in memory, which the reconciler's tests run on, and
// the one kept in MongoDB, which the control plane serves on, cannot drift
// apart.
//
// What it holds them to happens in time as their own clocks keep it: a lease
// taken for a few milliseconds expires once they pass, and a root backed off
// for a few is carried out again once they have.
package leasestest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Maker is a new repository of leases, with nothing kept in it yet, whose
// clock is the time now.
type Maker func(t *testing.T) graph.Leases

// The roots the suite takes the leases of: two, to hold them apart; and the
// control planes that take them.
const (
	shop  = "0193b5c2-7b1e-7000-8000-000000000001"
	cache = "0193b5c2-7b1e-7000-8000-000000000002"

	one     = "0193b5c2-7b1e-7000-8000-00000000000a"
	another = "0193b5c2-7b1e-7000-8000-00000000000b"
)

// loops is the reconcile loop's backoff, which the reconciler's is too.
var loops = graph.Backoff{First: 15 * time.Second, Most: 15 * time.Minute}

// settle is how long a lease that expires, or a backoff that ends, in a few
// milliseconds is waited on, and tick how often it is looked at meanwhile.
const (
	settle = 5 * time.Second
	tick   = 5 * time.Millisecond
)

// Repository fails t for everything the repository make makes does otherwise
// than every repository of leases has to.
func Repository(t *testing.T, make Maker) {
	t.Helper()

	t.Run("a lease nobody holds is taken, until as long from then as it is taken for, its times to the millisecond and in UTC", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		taken, err := repository.Take(ctx, shop, one, 30*time.Second)
		require.NoError(t, err)

		assert.Equal(t, shop, taken.Root)
		assert.Equal(t, one, taken.Holder)
		assert.Equal(t, 30*time.Second, taken.ExpiresAt.Sub(taken.UpdatedAt), "it expires as long after it is taken as it is taken for")
		assert.Zero(t, taken.Failures)
		assert.Empty(t, taken.Reason)
		assert.True(t, taken.NextAt.IsZero(), "nothing backs it off")

		kept(t, taken.ExpiresAt)
		kept(t, taken.UpdatedAt)
	})

	t.Run("one another holds is not taken while it lasts, and its holder takes it again, until as long again", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		held, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)
		assert.NotErrorIs(t, err, graph.ErrBackedOff)
		assert.Equal(t, one, held.Holder, "beside the lease as it is")

		again, err := repository.Take(ctx, shop, one, 2*time.Minute)
		require.NoError(t, err, "its holder takes it again")
		assert.Equal(t, one, again.Holder)
		assert.Equal(t, 2*time.Minute, again.ExpiresAt.Sub(again.UpdatedAt))
	})

	t.Run("asked for while another holds it, it is poked, which its holder's letting go of it says, and taken again it is not", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		taken, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)
		assert.False(t, taken.Poked, "a lease taken is poked by nothing")

		held, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)
		assert.True(t, held.Poked, "held, it is poked as it is asked for")
		assert.Equal(t, one, held.Holder)

		again, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err, "its holder takes it again")
		assert.False(t, again.Poked, "which a run that reads its root after it covers")

		_, err = repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)

		released, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		require.NoError(t, err)
		assert.True(t, released.Poked, "let go of, it says it was poked, for its holder to carry its root out again")
		assert.Empty(t, released.Holder)

		backedOff, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrBackedOff, "a poke holds back none of a backoff")
		assert.True(t, backedOff.Poked, "and is not taken away by it")

		require.NoError(t, repository.Forget(ctx, shop))

		forgotten, err := repository.Take(ctx, shop, another, time.Minute)
		require.NoError(t, err)
		assert.False(t, forgotten.Poked)

		read, err := repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.False(t, read.Poked, "a lease is poked by nothing but its being asked for while another holds it")
	})

	t.Run("taken now, one whose root is backed off is taken all the same, and what backs it off is cleared", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		held, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)
		require.True(t, held.Poked)

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops, Seen: graph.Seen{State: "starting", Since: time.Now()}})
		require.NoError(t, err)

		_, err = repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrBackedOff)

		taken, err := repository.TakeNow(ctx, shop, another, 30*time.Second)
		require.NoError(t, err)

		assert.Equal(t, another, taken.Holder)
		assert.Equal(t, 30*time.Second, taken.ExpiresAt.Sub(taken.UpdatedAt), "until as long from then as it is taken for")
		assert.Zero(t, taken.Failures, "its failures cleared")
		assert.Empty(t, taken.Reason)
		assert.True(t, taken.NextAt.IsZero(), "and nothing backs it off")
		assert.Equal(t, graph.Seen{}, taken.Seen)
		assert.False(t, taken.Poked, "the run it is taken for reads its root after whatever poked it")
		kept(t, taken.ExpiresAt)

		read, err := repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.Equal(t, taken, read, "as it is kept")

		again, err := repository.TakeNow(ctx, shop, another, time.Minute)
		require.NoError(t, err, "its holder takes it now again")
		assert.Equal(t, time.Minute, again.ExpiresAt.Sub(again.UpdatedAt))

		fresh, err := repository.TakeNow(ctx, cache, one, time.Minute)
		require.NoError(t, err, "and one never taken is taken now")
		assert.Equal(t, one, fresh.Holder)
		assert.Zero(t, fresh.Failures)
	})

	t.Run("but one another holds is not taken now, and is poked, as it is taken", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		held, err := repository.TakeNow(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)
		assert.NotErrorIs(t, err, graph.ErrBackedOff)
		assert.Equal(t, one, held.Holder, "beside the lease as it is")
		assert.True(t, held.Poked)

		released, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateDone})
		require.NoError(t, err)
		assert.True(t, released.Poked, "which its holder's letting go of it says")

		for name, take := range map[string]struct {
			root, holder string
			ttl          time.Duration
		}{
			"of no root":                {root: "", holder: one, ttl: time.Minute},
			"for nobody":                {root: shop, holder: "", ttl: time.Minute},
			"for less than millisecond": {root: shop, holder: one, ttl: 500 * time.Microsecond},
		} {
			_, err := repository.TakeNow(ctx, take.root, take.holder, take.ttl)
			assert.Error(t, err, name)
			assert.NotErrorIs(t, err, graph.ErrHeld, name)
		}
	})

	t.Run("of many that take a lease now at once, its root backed off, one takes it, and every other finds it held", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		require.NoError(t, err)

		var (
			start  sync.WaitGroup
			taking sync.WaitGroup
			lock   sync.Mutex
			took   []string
			held   int
		)

		// each waits for the rest, so that they take it at once.
		start.Add(1)

		for i := range 8 {
			holder := fmt.Sprintf("0193b5c2-7b1e-7000-8000-0000000001%02d", i)

			taking.Go(func() {
				start.Wait()

				_, err := repository.TakeNow(ctx, shop, holder, time.Minute)

				lock.Lock()
				defer lock.Unlock()

				switch {
				case err == nil:
					took = append(took, holder)
				case assert.ErrorIs(t, err, graph.ErrHeld):
					held++
				}
			})
		}

		start.Done()
		taking.Wait()

		require.Len(t, took, 1, "one takes it")
		assert.Equal(t, 7, held, "and every other finds it held")

		lease, err := repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.Equal(t, took[0], lease.Holder, "which holds it")
		assert.Zero(t, lease.Failures)
		assert.True(t, lease.Poked, "poked by the others")
	})

	t.Run("a run that failed keeps what it saw of its root, to the millisecond, which one that waits leaves and one done clears", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		since := time.Date(2026, 10, 11, 5, 0, 0, 123456789, time.FixedZone("Tehran", 12600))

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		failed, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Seen: graph.Seen{State: "starting", Since: since}})
		require.NoError(t, err)

		assert.Equal(t, graph.Seen{State: "starting", Since: time.Date(2026, 10, 11, 1, 30, 0, 123000000, time.UTC)}, failed.Seen)
		assert.True(t, failed.Seen.Is(graph.Seen{State: "starting", Since: since}), "the same state begun at the same time as it saw it")

		read, err := repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.Equal(t, failed.Seen, read.Seen, "as it is kept")

		_, err = repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		waited, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateWaiting, Reason: "its vm is stopped"})
		require.NoError(t, err)
		assert.Equal(t, failed.Seen, waited.Seen, "a run that waits leaves it, as it leaves the failure it is of")

		_, err = repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		blind, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "its record could not be read"})
		require.NoError(t, err)
		assert.Equal(t, graph.Seen{}, blind.Seen, "and one that saw nothing keeps nothing")

		_, err = repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Seen: graph.Seen{State: "stopping", Since: since}})
		require.NoError(t, err)

		_, err = repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		done, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateDone})
		require.NoError(t, err)
		assert.Equal(t, graph.Seen{}, done.Seen, "and a run done clears it")
	})

	t.Run("one that expired is taken by another, its failures as they were", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		// a failure that backs it off for nothing.
		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken"})
		require.NoError(t, err)

		_, err = repository.Take(ctx, shop, one, 50*time.Millisecond)
		require.NoError(t, err)

		var taken graph.Lease
		require.Eventually(t, func() bool {
			taken, err = repository.Take(ctx, shop, another, time.Minute)

			return err == nil
		}, settle, tick, "it is taken once it expires")

		assert.Equal(t, another, taken.Holder)
		assert.Equal(t, 1, taken.Failures)
		assert.Equal(t, "broken", taken.Reason)
	})

	t.Run("only its holder renews it, until as long from then as it is renewed for", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		renewed, err := repository.Renew(ctx, shop, one, 2*time.Minute)
		require.NoError(t, err)
		assert.Equal(t, one, renewed.Holder)
		assert.Equal(t, 2*time.Minute, renewed.ExpiresAt.Sub(renewed.UpdatedAt))
		kept(t, renewed.ExpiresAt)

		_, err = repository.Renew(ctx, shop, another, time.Hour)
		assert.ErrorIs(t, err, graph.ErrLost, "another does not hold it")

		held, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld, "nor did it renew it")
		assert.Equal(t, renewed.ExpiresAt, held.ExpiresAt)

		_, err = repository.Renew(ctx, cache, one, time.Minute)
		assert.ErrorIs(t, err, graph.ErrLost, "nobody holds one never taken")
	})

	t.Run("a run that failed lets go of it and backs its root off, from then, for as long as its backoff says, whoever asks for it", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		released, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		require.NoError(t, err)

		assert.Empty(t, released.Holder, "nobody holds it")
		assert.True(t, released.ExpiresAt.IsZero())
		assert.Equal(t, 1, released.Failures)
		assert.Equal(t, "broken", released.Reason)
		assert.Equal(t, 15*time.Second, released.NextAt.Sub(released.UpdatedAt), "backed off from when it ended")
		kept(t, released.NextAt)

		for _, holder := range []string{another, one} {
			backedOff, err := repository.Take(ctx, shop, holder, time.Minute)
			require.ErrorIs(t, err, graph.ErrBackedOff, "not taken while it is backed off")
			assert.NotErrorIs(t, err, graph.ErrHeld, "which is not its being held")
			assert.Equal(t, released, backedOff, "beside the lease as it is")
		}
	})

	t.Run("failures in a row back it off twice as long each time, up to the most", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		backoff := graph.Backoff{First: 20 * time.Millisecond, Most: 50 * time.Millisecond}

		for failures, want := range []time.Duration{20 * time.Millisecond, 40 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond} {
			// taken once the last backoff is over.
			require.Eventually(t, func() bool {
				_, err := repository.Take(ctx, shop, one, time.Minute)

				return err == nil
			}, settle, tick)

			released, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: backoff})
			require.NoError(t, err)

			assert.Equal(t, failures+1, released.Failures)
			assert.Equal(t, want, released.NextAt.Sub(released.UpdatedAt), "after %d failures", failures+1)
		}
	})

	t.Run("a run that waits leaves its failures as they were, and one done clears them", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		// a failure that backs it off for nothing, so that it is taken again
		// at once.
		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		failed, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken"})
		require.NoError(t, err)
		require.Equal(t, 1, failed.Failures)

		_, err = repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		waited, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateWaiting, Reason: "its vm is stopped", Backoff: loops})
		require.NoError(t, err)

		assert.Empty(t, waited.Holder, "nobody holds it")
		assert.True(t, waited.ExpiresAt.IsZero())
		assert.Equal(t, 1, waited.Failures, "a run that waits neither adds a failure nor clears one")
		assert.Equal(t, "broken", waited.Reason)
		assert.Equal(t, failed.NextAt, waited.NextAt)

		_, err = repository.Take(ctx, shop, another, time.Minute)
		require.NoError(t, err, "nor backs it off")

		done, err := repository.Release(ctx, shop, another, graph.Ended{State: graph.StateDone, Reason: "it was there", Backoff: loops})
		require.NoError(t, err)

		assert.Empty(t, done.Holder)
		assert.Zero(t, done.Failures, "a run done clears them")
		assert.Empty(t, done.Reason)
		assert.True(t, done.NextAt.IsZero())
		kept(t, done.UpdatedAt)
	})

	t.Run("only its holder lets go of it, and what is not an end lets go of nothing", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Release(ctx, shop, another, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		assert.ErrorIs(t, err, graph.ErrLost, "another does not hold it")

		for _, state := range []graph.State{graph.StatePending, graph.StateRunning, ""} {
			_, err = repository.Release(ctx, shop, one, graph.Ended{State: state, Backoff: loops})
			assert.Error(t, err, "%q is no end of a run", state)
			assert.NotErrorIs(t, err, graph.ErrLost)
		}

		held, err := repository.Take(ctx, shop, another, time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld, "it is held as it was")
		assert.Equal(t, one, held.Holder)
		assert.Zero(t, held.Failures)

		_, err = repository.Release(ctx, cache, one, graph.Ended{State: graph.StateDone})
		assert.ErrorIs(t, err, graph.ErrLost, "nobody holds one never taken")
	})

	t.Run("a forgotten lease is nobody's, and nothing of its failures is left", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		require.NoError(t, err)

		require.NoError(t, repository.Forget(ctx, shop))

		taken, err := repository.Take(ctx, shop, another, time.Minute)
		require.NoError(t, err, "neither backed off nor held")
		assert.Zero(t, taken.Failures)
		assert.Empty(t, taken.Reason)
		assert.True(t, taken.NextAt.IsZero())

		require.NoError(t, repository.Forget(ctx, shop), "forgotten while it is held")

		_, err = repository.Renew(ctx, shop, another, time.Minute)
		assert.ErrorIs(t, err, graph.ErrLost, "and lost to its holder")

		assert.NoError(t, repository.Forget(ctx, shop), "what is forgotten is forgotten already")
		assert.NoError(t, repository.Forget(ctx, cache), "and so is what was never taken")
	})

	t.Run("each root's lease is its own", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		_, err = repository.Take(ctx, cache, another, time.Minute)
		require.NoError(t, err, "another root's is free")

		_, err = repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: loops})
		require.NoError(t, err)

		_, err = repository.Take(ctx, shop, another, time.Minute)
		assert.ErrorIs(t, err, graph.ErrBackedOff)

		theirs, err := repository.Take(ctx, cache, another, time.Minute)
		require.NoError(t, err)
		assert.Zero(t, theirs.Failures, "and none of the other's failures are")

		require.NoError(t, repository.Forget(ctx, shop))

		_, err = repository.Renew(ctx, cache, another, time.Minute)
		assert.NoError(t, err, "nor is forgetting one forgetting another")
	})

	t.Run("a lease is read as it is kept, held or let go of, and taking nothing, and one there is none of is not there", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		_, err := repository.Get(ctx, shop)
		require.ErrorIs(t, err, domain.ErrNotExists)

		taken, err := repository.Take(ctx, shop, one, time.Minute)
		require.NoError(t, err)

		read, err := repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.Equal(t, taken, read, "held, as it was taken")

		released, err := repository.Release(ctx, shop, one, graph.Ended{State: graph.StateFailed, Reason: "its vm has no room", Backoff: loops})
		require.NoError(t, err)

		read, err = repository.Get(ctx, shop)
		require.NoError(t, err)
		assert.Equal(t, released, read, "let go of, its failure and its backoff kept")
		assert.Equal(t, 1, read.Failures)
		assert.Equal(t, "its vm has no room", read.Reason)
		assert.Empty(t, read.Holder)

		_, err = repository.Get(ctx, cache)
		assert.ErrorIs(t, err, domain.ErrNotExists, "each root's is its own")

		_, err = repository.Take(ctx, shop, another, time.Minute)
		assert.ErrorIs(t, err, graph.ErrBackedOff, "and reading it took nothing")

		require.NoError(t, repository.Forget(ctx, shop))

		_, err = repository.Get(ctx, shop)
		assert.ErrorIs(t, err, domain.ErrNotExists, "nor is one forgotten")
	})

	t.Run("a lease of no root, for nobody, or for less than a millisecond is not taken, nor anything else done to it", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		for name, take := range map[string]struct {
			root, holder string
			ttl          time.Duration
		}{
			"of no root":                {root: "", holder: one, ttl: time.Minute},
			"for nobody":                {root: shop, holder: "", ttl: time.Minute},
			"for no time":               {root: shop, holder: one, ttl: 0},
			"for less than none":        {root: shop, holder: one, ttl: -time.Minute},
			"for less than millisecond": {root: shop, holder: one, ttl: 500 * time.Microsecond},
		} {
			_, err := repository.Take(ctx, take.root, take.holder, take.ttl)
			assert.Error(t, err, name)
			assert.NotErrorIs(t, err, graph.ErrHeld, name)
			assert.NotErrorIs(t, err, graph.ErrBackedOff, name)
		}

		_, err := repository.Renew(ctx, "", one, time.Minute)
		assert.Error(t, err)

		_, err = repository.Release(ctx, "", one, graph.Ended{State: graph.StateDone})
		assert.Error(t, err)

		taken, err := repository.Take(ctx, shop, another, time.Minute)
		require.NoError(t, err, "nothing was taken")
		assert.Equal(t, another, taken.Holder)

		_, err = repository.Renew(ctx, shop, another, 0)
		assert.Error(t, err, "nor renewed for no time")
	})
}

// kept fails t unless a time is as a repository keeps it: to the
// millisecond, in UTC.
func kept(t *testing.T, at time.Time) {
	t.Helper()

	assert.False(t, at.IsZero(), "there is a time")
	assert.Equal(t, time.UTC, at.Location(), "in UTC")
	assert.Equal(t, at.Truncate(time.Millisecond), at, "to the millisecond")
}
