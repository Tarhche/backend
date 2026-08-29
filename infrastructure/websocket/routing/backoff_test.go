package routing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixedBackoff(t *testing.T) {
	t.Parallel()

	t.Run("allows the configured number of attempts in total", func(t *testing.T) {
		t.Parallel()

		backoff := NewFixedBackoff(3, time.Second)

		// attempt 1 has already been made when Next is asked about it, so three
		// attempts means Next agrees to two more and then stops.
		wait, retry := backoff.Next(1)
		assert.True(t, retry)
		assert.Equal(t, time.Second, wait)

		wait, retry = backoff.Next(2)
		assert.True(t, retry)
		assert.Equal(t, time.Second, wait)

		_, retry = backoff.Next(3)
		assert.False(t, retry, "the third attempt is the last one")
	})

	t.Run("a single attempt never retries", func(t *testing.T) {
		t.Parallel()

		_, retry := NewFixedBackoff(1, time.Second).Next(1)

		assert.False(t, retry)
	})
}
