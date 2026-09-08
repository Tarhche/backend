package gateway

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// cancellationsSubject is where a gateway announces that a client has stopped
// listening to a stream. Every replica sees it, because the client that walked
// away and the handler producing its stream need not be on the same one.
const cancellationsSubject = "streams_cancelled"

// StreamCancelled names a request whose client is no longer there to receive
// the rest of it.
type StreamCancelled struct {
	RequestID string `json:"request_id"`
}

// Streams is the set of streams one process is producing, so that a
// cancellation from any replica stops the right one. A handler that answers
// with more than a single reply registers its stream here and unregisters it
// when it ends:
//
//	ctx, cancel := context.WithCancel(ctx)
//	defer cancel()
//	streams.Add(requestID, cancel)
//	defer streams.Remove(requestID)
//
// It is itself the handler for the cancellations a gateway announces, so
// wiring it up is one call to Gateway.WatchStreamCancellations.
type Streams struct {
	lock sync.Mutex
	open map[string]context.CancelFunc
}

var _ domain.MessageHandler = &Streams{}

func NewStreams() *Streams {
	return &Streams{open: make(map[string]context.CancelFunc)}
}

// Add registers a stream under the request it answers. Registering the same
// request twice cancels the older stream, which is the only sensible reading of
// a second producer for one request.
func (s *Streams) Add(requestID string, cancel context.CancelFunc) {
	s.lock.Lock()
	previous, existed := s.open[requestID]
	s.open[requestID] = cancel
	s.lock.Unlock()

	if existed {
		previous()
	}
}

// Remove forgets a stream that has ended on its own.
func (s *Streams) Remove(requestID string) {
	s.lock.Lock()
	delete(s.open, requestID)
	s.lock.Unlock()
}

// Cancel stops a stream and reports whether this process was producing it.
func (s *Streams) Cancel(requestID string) bool {
	s.lock.Lock()
	cancel, ok := s.open[requestID]
	delete(s.open, requestID)
	s.lock.Unlock()

	if ok {
		cancel()
	}

	return ok
}

// Len reports how many streams this process is producing.
func (s *Streams) Len() int {
	s.lock.Lock()
	defer s.lock.Unlock()

	return len(s.open)
}

// Handle cancels the stream a gateway announced. A cancellation for a stream
// another replica is producing is not this one's business, so it is ignored.
func (s *Streams) Handle(ctx context.Context, payload []byte) error {
	var cancelled StreamCancelled

	if err := json.Unmarshal(payload, &cancelled); err != nil {
		// a malformed cancellation is not worth redelivering.
		return nil
	}

	s.Cancel(cancelled.RequestID)

	return nil
}
