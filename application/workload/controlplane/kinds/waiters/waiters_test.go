package waiters

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

func TestWaiters(t *testing.T) {
	t.Parallel()

	t.Run("whoever waits for a command is handed its result", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("command-1")
		defer wait.Done()

		go waiters.Answer(kind.Result{ID: "command-1", OK: true, Output: "started"})

		result, answered := wait.For(context.Background(), time.Minute, 0, nil)

		require.True(t, answered)
		assert.Equal(t, kind.Result{ID: "command-1", OK: true, Output: "started"}, result)
	})

	t.Run("an answer that comes before the wait begins to be waited on is still had", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("command-1")
		defer wait.Done()

		waiters.Answer(kind.Result{ID: "command-1", OK: true})

		_, answered := wait.For(context.Background(), time.Millisecond, 0, nil)

		assert.True(t, answered)
	})

	t.Run("another command's result is not the answer", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("command-1")
		defer wait.Done()

		waiters.Answer(kind.Result{ID: "command-2", OK: true})

		_, answered := wait.For(context.Background(), 20*time.Millisecond, 0, nil)

		assert.False(t, answered)
	})

	t.Run("nor is one nobody waits for anybody's", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		waiters.Answer(kind.Result{ID: "command-1", OK: true})

		assert.Equal(t, 0, waiters.Len())
	})

	t.Run("everybody waiting on one command hears it, and a redelivery changes nothing", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		first := waiters.Expect("command-1")
		defer first.Done()

		second := waiters.Expect("command-1")
		defer second.Done()

		waiters.Answer(kind.Result{ID: "command-1", OK: false, Reason: "it fell over"})
		waiters.Answer(kind.Result{ID: "command-1", OK: true})

		for _, wait := range []*Wait{first, second} {
			result, answered := wait.For(context.Background(), time.Second, 0, nil)

			require.True(t, answered)
			assert.Equal(t, "it fell over", result.Reason, "the first answer is the one had")
		}
	})

	t.Run("a wait is given up when its time runs out, or its caller goes", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("command-1")
		defer wait.Done()

		_, answered := wait.For(context.Background(), 10*time.Millisecond, 0, nil)
		assert.False(t, answered)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, answered = wait.For(ctx, time.Minute, 0, nil)
		assert.False(t, answered)
	})

	t.Run("an answer heard by another control plane is found where it was written down", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("command-1")
		defer wait.Done()

		var checked atomic.Int64

		result, answered := wait.For(context.Background(), time.Minute, time.Millisecond, func(context.Context) (kind.Result, bool) {
			if checked.Add(1) < 3 {
				return kind.Result{}, false
			}

			return kind.Result{ID: "command-1", OK: true, Output: "heard elsewhere"}, true
		})

		require.True(t, answered)
		assert.Equal(t, "heard elsewhere", result.Output)
		assert.Equal(t, int64(3), checked.Load())
	})

	t.Run("a command tried several times is answered by a result for any of its tries, once", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		wait := waiters.Expect("try-1", "try-2")
		assert.Equal(t, 1, waiters.Len(), "it is one wait")

		waiters.Answer(kind.Result{ID: "try-1", OK: true, Output: "the first try's"})
		waiters.Answer(kind.Result{ID: "try-2", OK: true, Output: "the second try's"})

		result, answered := wait.For(context.Background(), time.Second, 0, nil)
		require.True(t, answered)
		assert.Equal(t, "the first try's", result.Output)

		wait.Done()
		assert.Equal(t, 0, waiters.Len(), "and forgotten under every one of them")
	})

	t.Run("a wait that is done is forgotten", func(t *testing.T) {
		t.Parallel()

		waiters := New()

		var group sync.WaitGroup
		for range 10 {
			group.Go(func() {
				wait := waiters.Expect("command-1")
				defer wait.Done()

				waiters.Answer(kind.Result{ID: "command-1"})

				_, _ = wait.For(context.Background(), time.Second, 0, nil)
			})
		}

		group.Wait()

		assert.Equal(t, 0, waiters.Len())
	})
}
