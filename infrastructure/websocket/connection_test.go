package websocket

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
)

func TestConnectionShutdown(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("flushes what is already queued before closing", func(t *testing.T) {
		t.Parallel()

		const queued = 5

		// the handler queues messages and closes immediately, which is the
		// window in which a message can be lost: both the queue and the close
		// signal are ready, and the write pump may pick either.
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				return
			}

			config, err := newConfiguration(WithCloseGracePeriod(time.Second))
			if err != nil {
				return
			}

			c := newConnection(conn, config, logger)
			for i := range queued {
				c.send(&domain.Reply{RequestID: "queued", Payload: []byte{byte('0' + i)}})
			}

			c.shutdown()
		}))
		defer server.Close()

		u, err := url.Parse(server.URL)
		assert.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		assert.NoError(t, err)
		defer client.Close()

		assert.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))

		for i := range queued {
			var reply domain.Reply
			if err := client.ReadJSON(&reply); err != nil {
				t.Fatalf("message %d of %d was dropped at shutdown: %v", i+1, queued, err)
			}

			assert.Equal(t, "queued", reply.RequestID)
		}
	})

	t.Run("is bounded by the close grace period when the client never reads", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithWriteWait(50*time.Millisecond),
			WithCloseGracePeriod(100*time.Millisecond),
		)
		assert.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, logger)

		c.send(&domain.Reply{RequestID: "1"})
		c.send(&domain.Reply{RequestID: "2"})

		start := time.Now()
		c.shutdown()

		assert.Less(t, time.Since(start), time.Second, "shutdown waited on a client that was never going to read")
	})

	t.Run("is safe to call more than once", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithWriteWait(50*time.Millisecond),
			WithCloseGracePeriod(50*time.Millisecond),
		)
		assert.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, logger)

		assert.NotPanics(t, func() {
			c.shutdown()
			c.shutdown()
		})

		assert.False(t, c.send(&domain.Reply{RequestID: "1"}), "a closed connection should refuse messages")
	})
}
