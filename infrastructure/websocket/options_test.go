package websocket

import (
	"net/http"
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
		assert.Equal(t, int64(defaultMaxMessageSize), config.maxMessageSize)
		assert.Equal(t, defaultWriteWait, config.writeWait)
		assert.Equal(t, defaultPingPeriod, config.pingPeriod)
		assert.Equal(t, defaultPongWait, config.pongWait)
		assert.Equal(t, defaultOutboundBuffer, config.outboundBuffer)
		assert.Equal(t, defaultCloseGracePeriod, config.closeGracePeriod)
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
			WithOriginChecker(func(*http.Request) bool { return false }),
		)

		assert.NoError(t, err)
		assert.Equal(t, int64(256*1024), config.maxMessageSize)
		assert.Equal(t, 10*time.Second, config.writeWait)
		assert.Equal(t, 60*time.Second, config.pongWait)
		assert.Equal(t, 54*time.Second, config.pingPeriod)
		assert.Equal(t, 32, config.outboundBuffer)
		assert.Equal(t, 5*time.Second, config.closeGracePeriod)
		assert.False(t, config.checkOrigin(&http.Request{}))
	})

	testcases := map[string]Option{
		// pinging no more often than the client is given to answer would
		// disconnect it between pings.
		"a ping period that is not shorter than the pong wait": WithPingPeriod(defaultPongWait),
		"a message size limit of zero":                         WithMaxMessageSize(0),
		"a write wait of zero":                                 WithWriteWait(0),
		"a negative outbound buffer":                           WithOutboundBuffer(-1),
		// at zero, a reply only lands when the pump happens to be parked on the
		// receive, so delivery becomes a coin flip.
		"an outbound buffer of zero":    WithOutboundBuffer(0),
		"a negative close grace period": WithCloseGracePeriod(-time.Second),
		"no origin checker":             WithOriginChecker(nil),
	}

	for name, option := range testcases {
		t.Run(name+" is rejected", func(t *testing.T) {
			t.Parallel()

			_, err := newConfiguration(option)

			assert.Error(t, err)
		})
	}
}
