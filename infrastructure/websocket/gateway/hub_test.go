package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub(t *testing.T) {
	t.Parallel()

	// newTestSession builds only as much of a session as the hub touches.
	newTestSession := func(buffer int) *session {
		return &session{
			conn:         newIdleConn(),
			registry:     NewInMemoryRequestRegistry(0),
			cancelStream: func(context.Context, string) {},
			replies:      make(chan *domain.Reply, buffer),
			done:         make(chan struct{}),
			gone:         make(chan struct{}),
			logger:       discardLogger(),
		}
	}

	t.Run("a reply reaches every session", func(t *testing.T) {
		t.Parallel()

		h := newHub(discardLogger())

		first, second := newTestSession(1), newTestSession(1)

		_, joined := h.join(first)
		require.True(t, joined)
		_, joined = h.join(second)
		require.True(t, joined)

		assert.Equal(t, 2, h.size())

		reply := &domain.Reply{RequestID: "1"}
		h.broadcast(reply)

		assert.Same(t, reply, <-first.replies)
		assert.Same(t, reply, <-second.replies)
	})

	t.Run("leaving removes the session and closes its queue", func(t *testing.T) {
		t.Parallel()

		h := newHub(discardLogger())

		s := newTestSession(1)

		leave, joined := h.join(s)
		require.True(t, joined)

		leave()
		assert.Equal(t, 0, h.size())

		_, open := <-s.replies
		assert.False(t, open, "the queue was left open, so writeReplies would never return")

		assert.NotPanics(t, leave, "leaving twice would close the queue twice")
	})

	t.Run("a session that is not keeping up is skipped, not waited on", func(t *testing.T) {
		t.Parallel()

		h := newHub(discardLogger())

		s := newTestSession(1)

		_, joined := h.join(s)
		require.True(t, joined)

		done := make(chan struct{})
		go func() {
			defer close(done)

			// one more than the queue holds
			h.broadcast(&domain.Reply{RequestID: "1"})
			h.broadcast(&domain.Reply{RequestID: "2"})
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("broadcast blocked on a session whose queue was full")
		}

		assert.Len(t, s.replies, 1)
	})

	t.Run("closing stops every session and refuses further joins", func(t *testing.T) {
		t.Parallel()

		h := newHub(discardLogger())

		sessions := make([]*session, 0, 3)
		for range 3 {
			s := newTestSession(1)
			s.hub = h

			go s.run(context.Background())

			sessions = append(sessions, s)
		}

		require.Eventually(t, func() bool { return h.size() == 3 }, 2*time.Second, 5*time.Millisecond)

		h.closeAll()

		assert.Equal(t, 0, h.size())

		for i, s := range sessions {
			select {
			case <-s.gone:
			default:
				t.Fatalf("session %d was still running after the hub closed", i)
			}
		}

		_, joined := h.join(newTestSession(1))
		assert.False(t, joined, "a client accepted during a shutdown would never be stopped")
	})
}
