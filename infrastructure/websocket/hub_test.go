package websocket

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
)

func TestHub(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("a reply reaches every subscriber", func(t *testing.T) {
		t.Parallel()

		h := newHub(1, logger)

		first, _ := h.subscribe()
		second, _ := h.subscribe()

		reply := &domain.Reply{RequestID: "server-1"}
		h.broadcast(reply)

		assert.Equal(t, reply, <-first)
		assert.Equal(t, reply, <-second)
	})

	t.Run("unsubscribing removes and closes the channel", func(t *testing.T) {
		t.Parallel()

		h := newHub(1, logger)

		replies, unsubscribe := h.subscribe()
		assert.Equal(t, 1, h.size())

		unsubscribe()
		assert.Equal(t, 0, h.size())

		_, open := <-replies
		assert.False(t, open, "the channel should be closed so its reader can stop")

		// the connection teardown path may run more than once; it must stay safe.
		assert.NotPanics(t, unsubscribe)
	})

	t.Run("a subscriber that is not keeping up is skipped, not waited on", func(t *testing.T) {
		t.Parallel()

		h := newHub(1, logger)

		slow, _ := h.subscribe()
		fast, _ := h.subscribe()

		// fill the slow subscriber's buffer and leave it unread.
		h.broadcast(&domain.Reply{RequestID: "server-1"})
		<-fast

		broadcast := make(chan struct{})
		go func() {
			defer close(broadcast)

			h.broadcast(&domain.Reply{RequestID: "server-2"})
		}()

		select {
		case <-broadcast:
		case <-time.After(time.Second):
			t.Fatal("broadcast blocked on a subscriber that is not reading")
		}

		// the reply the slow subscriber missed still reached the other one.
		assert.Equal(t, "server-2", (<-fast).RequestID)
		assert.Equal(t, "server-1", (<-slow).RequestID)
	})
}
