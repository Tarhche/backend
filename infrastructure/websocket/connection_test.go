package websocket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnection(t *testing.T) {
	t.Parallel()

	t.Run("flushes what is already queued before closing", func(t *testing.T) {
		t.Parallel()

		const queued = 5

		// the handler queues replies and closes immediately, which is the
		// window in which a reply can be lost: both the queue and the close
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

			c := newConnection(conn, config, discardLogger())
			for i := range queued {
				c.Send(&domain.Reply{RequestID: "queued", Payload: []byte{byte('0' + i)}})
			}

			c.Close()
		}))
		defer server.Close()

		u, err := url.Parse(server.URL)
		require.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer client.Close()

		require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))

		for i := range queued {
			var reply domain.Reply
			if err := client.ReadJSON(&reply); err != nil {
				t.Fatalf("reply %d of %d was dropped at shutdown: %v", i+1, queued, err)
			}

			assert.Equal(t, "queued", reply.RequestID)
		}
	})

	t.Run("closing is bounded when the client never reads", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithWriteWait(50*time.Millisecond),
			WithCloseGracePeriod(100*time.Millisecond),
		)
		require.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, discardLogger())

		c.Send(&domain.Reply{RequestID: "1"})
		c.Send(&domain.Reply{RequestID: "2"})

		start := time.Now()
		c.Close()

		// the grace period bounds the queue, and one write in flight may run on
		// past it for as long as the write wait allows.
		assert.Less(t, time.Since(start), time.Second, "closing waited on a client that was never going to read")
	})

	t.Run("closing waits for a write that is already in flight", func(t *testing.T) {
		t.Parallel()

		// a write that outlives the grace period must not have the socket
		// closed underneath it, or the reply the drain exists to deliver is
		// truncated instead.
		config, err := newConfiguration(
			WithWriteWait(300*time.Millisecond),
			WithCloseGracePeriod(20*time.Millisecond),
		)
		require.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, discardLogger())

		require.True(t, c.Send(&domain.Reply{RequestID: "1"}))

		// let the pump pick the reply up and block on the stalled client
		time.Sleep(50 * time.Millisecond)

		start := time.Now()
		c.Close()

		assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond,
			"the socket was closed while the pump was still writing")
	})

	t.Run("is safe to close more than once", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithWriteWait(50*time.Millisecond),
			WithCloseGracePeriod(50*time.Millisecond),
		)
		require.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, discardLogger())

		assert.NotPanics(t, func() {
			c.Close()
			c.Close()
		})

		assert.False(t, c.Send(&domain.Reply{RequestID: "1"}), "a closed connection should refuse replies")
	})

	t.Run("refuses a reply once the client's queue is full", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithOutboundBuffer(1),
			WithWriteWait(50*time.Millisecond),
			WithCloseGracePeriod(10*time.Millisecond),
		)
		require.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, discardLogger())
		defer c.Close()

		// the pump takes the first, the buffer holds the second, and the client
		// is not reading, so everything after that has nowhere to go.
		var refused bool
		for range 10 {
			if !c.Send(&domain.Reply{RequestID: "1"}) {
				refused = true

				break
			}
		}

		assert.True(t, refused, "a client that never reads was allowed to queue without limit")
	})

	t.Run("reports a connection that ended as the end of the conversation", func(t *testing.T) {
		t.Parallel()

		config, err := newConfiguration(
			WithWriteWait(20*time.Millisecond),
			WithCloseGracePeriod(10*time.Millisecond),
		)
		require.NoError(t, err)

		c := newConnection(stalledClientConn(t), config, discardLogger())
		c.Close()

		// io.EOF is what tells the gateway a client said goodbye rather than
		// that the transport broke, which is the difference between an info
		// line and an error in the logs.
		assert.ErrorIs(t, c.Read(&domain.Request{}), io.EOF)
	})

	t.Run("keeps pinging a client that is otherwise silent", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				return
			}

			config, err := newConfiguration(WithPingPeriod(20*time.Millisecond), WithPongWait(time.Second))
			if err != nil {
				return
			}

			c := newConnection(conn, config, discardLogger())
			defer c.Close()

			// hold the connection open long enough for a few pings
			time.Sleep(200 * time.Millisecond)
		}))
		defer server.Close()

		u, err := url.Parse(server.URL)
		require.NoError(t, err)
		u.Scheme = "ws"

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer client.Close()

		pinged := make(chan struct{}, 8)
		client.SetPingHandler(func(string) error {
			select {
			case pinged <- struct{}{}:
			default:
			}

			return nil
		})

		require.NoError(t, client.SetReadDeadline(time.Now().Add(2*time.Second)))

		go func() {
			for {
				if _, _, err := client.ReadMessage(); err != nil {
					return
				}
			}
		}()

		select {
		case <-pinged:
		case <-time.After(2 * time.Second):
			t.Fatal("the client was never pinged, so its read deadline would lapse")
		}
	})
}
