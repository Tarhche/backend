package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("defaults to a working configuration", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration()

		assert.NoError(t, err)
		assert.Equal(t, defaultSubjectPrefix, config.subjectPrefix)
		assert.Equal(t, defaultReplyBuffer, config.replyBuffer)
		assert.Equal(t, defaultMaxInFlightRequests, config.maxInFlightRequests)
		assert.Equal(t, NewFixedBackoff(defaultReplyAttempts, defaultReplyWait), config.replyBackoff)
		assert.Equal(t, NewFixedBackoff(defaultQueueAttempts, defaultQueueWait), config.queueBackoff)
		assert.NotNil(t, config.registries)
		assert.Equal(t, 0, config.registries().Len(), "every connection starts with an empty registry")
	})

	t.Run("applies the given options", func(t *testing.T) {
		t.Parallel()

		registry := NewInMemoryRequestRegistry(1)

		config, err := newConfiguration(
			WithSubjectPrefix("tcp_"),
			WithReplyBuffer(32),
			WithMaxInFlightRequests(3),
			WithReplyBackoff(NewFixedBackoff(7, time.Minute)),
			WithQueueBackoff(NewFixedBackoff(2, time.Second)),
			WithRequestRegistry(func() RequestRegistry { return registry }),
		)

		assert.NoError(t, err)
		assert.Equal(t, "tcp_", config.subjectPrefix)
		assert.Equal(t, 32, config.replyBuffer)
		assert.Equal(t, 3, config.maxInFlightRequests)
		assert.Equal(t, NewFixedBackoff(7, time.Minute), config.replyBackoff)
		assert.Equal(t, NewFixedBackoff(2, time.Second), config.queueBackoff)
		assert.Same(t, registry, config.registries())
	})

	testcases := map[string]Option{
		// at zero, a reply only lands when a session happens to be parked on
		// the receive, so delivery becomes a coin flip.
		"a reply buffer of zero":      WithReplyBuffer(0),
		"a negative reply buffer":     WithReplyBuffer(-1),
		"an in-flight limit of zero":  WithMaxInFlightRequests(0),
		"a negative in-flight limit":  WithMaxInFlightRequests(-1),
		"no reply backoff":            WithReplyBackoff(nil),
		"no queue backoff":            WithQueueBackoff(nil),
		"no request registry factory": WithRequestRegistry(nil),
	}

	for name, option := range testcases {
		t.Run(name+" is rejected", func(t *testing.T) {
			t.Parallel()

			_, err := newConfiguration(option)

			assert.Error(t, err)
		})
	}
}
