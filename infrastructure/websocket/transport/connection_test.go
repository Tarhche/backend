package transport

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
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/websockettest"
	"github.com/stretchr/testify/assert"
)

// testConfig is a working socket configuration with the given overrides applied.
func testConfig(overrides ...func(*Config)) Config {
	config := Config{
		MaxMessageSize:   1024 * 10,
		WriteWait:        6 * time.Second,
		PingPeriod:       2 * time.Second,
		PongWait:         6 * time.Second,
		OutboundBuffer:   10,
		CloseGracePeriod: time.Second,
	}

	for _, override := range overrides {
		override(&config)
	}

	return config
}

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

			c := NewConnection(conn, testConfig(func(c *Config) {
				c.CloseGracePeriod = time.Second
			}), logger)
			for i := range queued {
				c.Send(&domain.Reply{RequestID: "queued", Payload: []byte{byte('0' + i)}})
			}

			c.Shutdown()
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

		c := NewConnection(websockettest.StalledClientConn(t), testConfig(func(c *Config) {
			c.WriteWait = 50 * time.Millisecond
			c.CloseGracePeriod = 100 * time.Millisecond
		}), logger)

		c.Send(&domain.Reply{RequestID: "1"})
		c.Send(&domain.Reply{RequestID: "2"})

		start := time.Now()
		c.Shutdown()

		assert.Less(t, time.Since(start), time.Second, "shutdown waited on a client that was never going to read")
	})

	t.Run("is safe to call more than once", func(t *testing.T) {
		t.Parallel()

		c := NewConnection(websockettest.StalledClientConn(t), testConfig(func(c *Config) {
			c.WriteWait = 50 * time.Millisecond
			c.CloseGracePeriod = 50 * time.Millisecond
		}), logger)

		assert.NotPanics(t, func() {
			c.Shutdown()
			c.Shutdown()
		})

		assert.False(t, c.Send(&domain.Reply{RequestID: "1"}), "a closed connection should refuse messages")
	})
	t.Run("a stalled client fills its own queue instead of blocking the sender", func(t *testing.T) {
		t.Parallel()

		config := testConfig(func(c *Config) {
			c.WriteWait = 50 * time.Millisecond
			c.OutboundBuffer = 2
		})

		conn := NewConnection(websockettest.StalledClientConn(t), config, logger)
		defer conn.Shutdown()

		// the client never reads, so the queue fills and then refuses more
		// rather than waiting for room: at most its buffer plus the one
		// message the write pump is stuck writing.
		const attempts = 10

		accepted := make(chan int, 1)
		go func() {
			count := 0
			for range attempts {
				if conn.Send(&domain.Reply{RequestID: "server-1"}) {
					count++
				}
			}
			accepted <- count
		}()

		select {
		case count := <-accepted:
			assert.LessOrEqual(t, count, config.OutboundBuffer+1, "send accepted more than the outbound queue can hold")
			assert.Less(t, count, attempts, "send should start refusing messages once the queue is full")
		case <-time.After(time.Second):
			t.Fatal("send blocked on a client that never reads")
		}
	})
}
