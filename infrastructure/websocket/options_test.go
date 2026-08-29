package websocket

import (
	"net/http"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/websocket/routing"
	"github.com/stretchr/testify/assert"
)

func TestConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("defaults to a working configuration", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration()

		assert.NoError(t, err)
		assert.Equal(t, int64(defaultMaxMessageSize), config.maxMessageSize)
		assert.Equal(t, defaultWriteWait, config.writeWait)
		assert.Equal(t, defaultPingPeriod, config.pingPeriod)
		assert.Equal(t, defaultPongWait, config.pongWait)
		assert.Equal(t, defaultOutboundBuffer, config.outboundBuffer)
		assert.Equal(t, defaultCloseGracePeriod, config.closeGracePeriod)
		assert.Equal(t, routing.DefaultReplyBackoff(), config.replyBackoff)
		assert.Equal(t, routing.DefaultQueueBackoff(), config.queueBackoff)
		assert.True(t, config.checkOrigin(&http.Request{}), "the default accepts every origin")
	})

	t.Run("applies the given options", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithMaxMessageSize(256*1024),
			WithWriteWait(10*time.Second),
			WithPongWait(60*time.Second),
			WithPingPeriod(54*time.Second),
			WithOutboundBuffer(32),
			WithCloseGracePeriod(5*time.Second),
			WithReplyBackoff(NewFixedBackoff(7, time.Minute)),
			WithOriginChecker(func(*http.Request) bool { return false }),
		)

		assert.NoError(t, err)
		assert.Equal(t, int64(256*1024), config.maxMessageSize)
		assert.Equal(t, 10*time.Second, config.writeWait)
		assert.Equal(t, 60*time.Second, config.pongWait)
		assert.Equal(t, 54*time.Second, config.pingPeriod)
		assert.Equal(t, 32, config.outboundBuffer)
		assert.Equal(t, 5*time.Second, config.closeGracePeriod)
		assert.Equal(t, NewFixedBackoff(7, time.Minute), config.replyBackoff)
		assert.False(t, config.checkOrigin(&http.Request{}))
	})

	testcases := []struct {
		name   string
		option Option
	}{
		{
			// pinging no more often than the client is given to answer would
			// disconnect it between pings.
			name:   "a ping period that is not shorter than the pong wait",
			option: WithPingPeriod(defaultPongWait),
		},
		{
			name:   "a message size limit of zero",
			option: WithMaxMessageSize(0),
		},
		{
			name:   "a write wait of zero",
			option: WithWriteWait(0),
		},
		{
			name:   "a negative outbound buffer",
			option: WithOutboundBuffer(-1),
		},
		{
			// at zero, a reply only lands when a session happens to be parked
			// on the receive, so delivery becomes a coin flip.
			name:   "an outbound buffer of zero",
			option: WithOutboundBuffer(0),
		},
		{
			name:   "a negative close grace period",
			option: WithCloseGracePeriod(-time.Second),
		},
		{
			name:   "no reply backoff",
			option: WithReplyBackoff(nil),
		},
		{
			name:   "no queue backoff",
			option: WithQueueBackoff(nil),
		},
		{
			name:   "no origin checker",
			option: WithOriginChecker(nil),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name+" is rejected", func(t *testing.T) {
			t.Parallel()

			_, err := newConfiguration(testcase.option)

			assert.Error(t, err)
		})
	}
}
