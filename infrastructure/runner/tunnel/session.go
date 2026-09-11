package tunnel

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/xtaci/smux"
)

var (
	// ErrSessionClosed is a session that has gone since it was picked.
	ErrSessionClosed = errors.New("tunnel: session is closed")

	// ErrAtCapacity is a session, or a whole worker, carrying as many streams
	// as it is allowed to.
	ErrAtCapacity = errors.New("tunnel: at capacity")
)

// Session is one of a worker's TCP connections, and the smux session over it.
//
// It is the unit of failure: everything on it dies with it, and nothing else
// does. It is also the unit of capacity — how many streams it will carry is
// settled here rather than anywhere that would have to be asked.
type Session struct {
	id     string
	worker string

	session *smux.Session
	created time.Time

	// maxStreams is what this session will carry. Reserving against it is what
	// keeps two callers from each taking the last place.
	maxStreams int
	streams    atomic.Int64

	// sent and received are what has gone through this session, for reporting
	// load that a stream count cannot see.
	sent     atomic.Int64
	received atomic.Int64
}

func newSession(id string, worker string, session *smux.Session, maxStreams int) *Session {
	return &Session{
		id:         id,
		worker:     worker,
		session:    session,
		created:    time.Now(),
		maxStreams: maxStreams,
	}
}

// ID names this session within its worker, so a stream can be traced back to
// the connection that carried it.
func (s *Session) ID() string { return s.id }

// Worker is whose session this is.
func (s *Session) Worker() string { return s.worker }

// CreatedAt is when the connection was made.
func (s *Session) CreatedAt() time.Time { return s.created }

// Streams is how many are on it now. It is the reserved count rather than
// smux's own, because a stream is reserved before it exists and released after
// it is gone — counting smux's would let a burst overshoot the limit.
func (s *Session) Streams() int { return int(s.streams.Load()) }

// Capacity is how many it will carry.
func (s *Session) Capacity() int { return s.maxStreams }

// Free is how many more it will take.
func (s *Session) Free() int {
	free := s.maxStreams - s.Streams()
	if free < 0 {
		return 0
	}

	return free
}

// Traffic is what has gone through, out and back.
func (s *Session) Traffic() (sent int64, received int64) {
	return s.sent.Load(), s.received.Load()
}

// Closed reports a session whose connection has gone.
func (s *Session) Closed() bool { return s.session.IsClosed() }

// Usable reports a session that can still be given a stream.
func (s *Session) Usable() bool { return !s.Closed() && s.Free() > 0 }

// Done is closed when the session is, so a watcher does not have to poll.
func (s *Session) Done() <-chan struct{} { return s.session.CloseChan() }

// Close ends the session and everything on it.
func (s *Session) Close() error { return s.session.Close() }

// reserve takes one of the session's places, or reports that there were none.
// It is a compare-and-swap rather than a lock because it is on the path of
// every client connection, and because the only thing it guards is a number.
func (s *Session) reserve() bool {
	for {
		current := s.streams.Load()
		if int(current) >= s.maxStreams {
			return false
		}

		if s.streams.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (s *Session) release() { s.streams.Add(-1) }

func (s *Session) account(sent int64, received int64) {
	s.sent.Add(sent)
	s.received.Add(received)
}

// open starts a stream on this session and asks the worker to connect it to the
// target. The place on the session has to have been reserved already.
//
// The stream comes back only once the worker has said it reached the target, so
// what the caller is given is a pipe that is known to lead somewhere.
func (s *Session) open(ctx context.Context, target Target, timeout time.Duration) (net.Conn, error) {
	stream, err := s.session.OpenStream()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	if fromContext, ok := ctx.Deadline(); ok && fromContext.Before(deadline) {
		deadline = fromContext
	}

	if err := writeStreamFrame(stream, target, deadline); err != nil {
		stream.Close()

		return nil, err
	}

	var answer opened
	leftover, err := readStreamFrame(stream, &answer, deadline)
	if err != nil {
		stream.Close()

		return nil, err
	}

	if !answer.OK {
		stream.Close()

		return nil, rejection(answer.Error)
	}

	if err := stream.SetDeadline(time.Time{}); err != nil {
		stream.Close()

		return nil, err
	}

	return withPrefix(stream, leftover), nil
}

// smuxServerOn starts a session over a connection with no handshake, which is
// what the tests that are about counting rather than carrying need.
func smuxServerOn(conn net.Conn) (*smux.Session, error) {
	return smux.Server(conn, DefaultConfig().smux())
}
