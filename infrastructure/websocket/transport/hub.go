package transport

import (
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// Hub fans a reply out to every client connected to this replica. Recognising
// which reply belongs to which client is the session's job, not the Hub's.
type Hub struct {
	lock        sync.RWMutex
	subscribers map[chan *domain.Reply]struct{}
	buffer      int
	logger      *slog.Logger
}

func NewHub(buffer int, logger *slog.Logger) *Hub {
	return &Hub{
		subscribers: make(map[chan *domain.Reply]struct{}, 10),
		buffer:      buffer,
		logger:      logger,
	}
}

// subscribe registers a reply channel and returns it with the function that
// unregisters and closes it. Unsubscribing twice is a no-op.
func (h *Hub) Subscribe() (<-chan *domain.Reply, func()) {
	replies := make(chan *domain.Reply, h.buffer)

	h.lock.Lock()
	h.subscribers[replies] = struct{}{}
	h.lock.Unlock()

	var once sync.Once

	return replies, func() {
		once.Do(func() {
			// remove before closing: broadcast only sends on channels it
			// found while holding the lock.
			h.lock.Lock()
			delete(h.subscribers, replies)
			h.lock.Unlock()

			close(replies)
		})
	}
}

// broadcast delivers a reply to every subscriber with room for it, skipping
// those whose buffer is full rather than waiting on them.
func (h *Hub) Broadcast(reply *domain.Reply) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	for replies := range h.subscribers {
		select {
		case replies <- reply:
		default:
			h.logger.Warn("response channel is full due to slow connection, skipping the reply", "requestID", reply.RequestID)
		}
	}
}

func (h *Hub) Size() int {
	h.lock.RLock()
	defer h.lock.RUnlock()

	return len(h.subscribers)
}
