package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_MayRetry(t *testing.T) {
	t.Parallel()

	t.Run("a container is worth as many attempts as it asked for", func(t *testing.T) {
		t.Parallel()

		container := Task{MaxRetries: 2}

		assert.True(t, container.MayRetry(0))
		assert.True(t, container.MayRetry(1))
		assert.False(t, container.MayRetry(2))
	})

	t.Run("a container worth none is not tried again", func(t *testing.T) {
		t.Parallel()

		assert.False(t, (&Task{MaxRetries: 0}).MayRetry(0))
	})

	t.Run("a container the runner never gives up on is always worth another", func(t *testing.T) {
		t.Parallel()

		container := Task{MaxRetries: RetryForever}

		assert.True(t, container.MayRetry(0))
		assert.True(t, container.MayRetry(1000))
	})
}

func TestTask_Attempt(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("a container that failed on its way up keeps the count", func(t *testing.T) {
		t.Parallel()

		container := Task{StartedAt: now.Add(-time.Second)}

		assert.Equal(t, 2, container.Attempt(2, now))
	})

	t.Run("a container that stood for a while starts the count again", func(t *testing.T) {
		t.Parallel()

		container := Task{StartedAt: now.Add(-RetryWindow - time.Second)}

		assert.Equal(t, 0, container.Attempt(2, now))
	})

	t.Run("a container that never started keeps the count", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, 3, (&Task{}).Attempt(3, now))
	})
}

func TestDefaultMaxRetries(t *testing.T) {
	t.Parallel()

	// a job is asked for once, by somebody waiting for what it says.
	assert.Equal(t, 0, DefaultMaxRetries(KindJob))
	assert.Positive(t, DefaultMaxRetries(KindService))
}

func TestRetryDelay(t *testing.T) {
	t.Parallel()

	// even the first failure is left a moment, so that what happens to a
	// container can be seen happening.
	assert.Positive(t, RetryDelay(0))

	// and what keeps failing is waited on longer each time, up to a point.
	assert.Less(t, RetryDelay(0), RetryDelay(1))
	assert.Less(t, RetryDelay(1), RetryDelay(2))
	assert.Equal(t, RetryDelay(1000), RetryDelay(10000))
}

func TestTask_RetryDue(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("a container that has just failed is left alone", func(t *testing.T) {
		t.Parallel()

		container := Task{ExpectedState: Running, FinishedAt: now}

		assert.False(t, container.RetryDue(now, 3))
	})

	t.Run("a container left long enough is worth another attempt", func(t *testing.T) {
		t.Parallel()

		container := Task{ExpectedState: Running, FinishedAt: now.Add(-time.Minute)}

		assert.True(t, container.RetryDue(now, 3))
	})

	t.Run("a container nobody wants running is not worth one", func(t *testing.T) {
		t.Parallel()

		container := Task{ExpectedState: Stopped, FinishedAt: now.Add(-time.Minute)}

		assert.False(t, container.RetryDue(now, 3))
	})
}
