package gateway

import (
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// hub is the set of sessions this replica is serving. It hands every reply to
// all of them — recognising which reply belongs to which client is a session's
// job, not the hub's — and can disconnect them all at once.
type hub struct {
	lock     sync.RWMutex
	sessions map[*session]struct{}
	closed   bool
	logger   *slog.Logger
}

func newHub(logger *slog.Logger) *hub {
	return &hub{
		sessions: make(map[*session]struct{}, 10),
		logger:   logger,
	}
}

// join registers a session and returns the function that removes it and closes
// its reply queue. Leaving twice is a no-op.
//
// It reports false once the hub has been closed, so a client accepted in the
// window before a shutdown is not left in a hub that will never stop it.
func (h *hub) join(s *session) (func(), bool) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if h.closed {
		return nil, false
	}

	h.sessions[s] = struct{}{}

	var once sync.Once

	return func() {
		once.Do(func() {
			// remove before closing: broadcast only sends on the queues it
			// found while holding the lock.
			h.lock.Lock()
			delete(h.sessions, s)
			h.lock.Unlock()

			close(s.replies)
		})
	}, true
}

// broadcast delivers a reply to every session with room for it, skipping those
// whose queue is full rather than waiting on them.
func (h *hub) broadcast(reply *domain.Reply) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	for s := range h.sessions {
		select {
		case s.replies <- reply:
		default:
			h.logger.Warn("a session's reply queue is full due to a slow connection, skipping the reply", "requestID", reply.RequestID)
		}
	}
}

// closeAll disconnects every session and refuses further joins. Sessions are
// stopped concurrently, so a shutdown costs the slowest connection rather than
// the sum of all of them, and it returns only once they are all gone.
func (h *hub) closeAll() {
	h.lock.Lock()
	h.closed = true
	sessions := make([]*session, 0, len(h.sessions))
	for s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.lock.Unlock()

	var group sync.WaitGroup

	for _, s := range sessions {

		group.Go(func() {

			s.stop()
		})
	}

	group.Wait()
}

func (h *hub) size() int {
	h.lock.RLock()
	defer h.lock.RUnlock()

	return len(h.sessions)
}
