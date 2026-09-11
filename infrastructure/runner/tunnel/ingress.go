package tunnel

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/xtaci/smux"
)

// Ingress takes the workers' connections and opens streams back down them.
//
// It is deliberately three things kept apart: taking a connection and
// registering it, deciding which worker a client belongs to, and copying bytes.
// Only the first two touch shared state, and only the registry is shared at
// all — which is what leaves room for a second ingress to be given a registry
// that both can see, without the rest of this changing.
type Ingress struct {
	config   Config
	registry Registry
	auth     Authenticator
	router   Router
	metrics  Metrics
	logger   *slog.Logger

	closeOnce sync.Once
	closed    chan struct{}

	// sessions are the ones this ingress is serving, so that closing it closes
	// them rather than leaving workers talking to nothing. closing is set
	// before anything waits, because a WaitGroup counted up from zero while
	// something is already waiting on it is a race rather than a queue.
	lock     sync.Mutex
	closing  bool
	sessions map[*Session]struct{}

	wait sync.WaitGroup
}

// IngressOption configures an Ingress.
type IngressOption func(*Ingress)

// WithRouter replaces the routing policy.
func WithRouter(router Router) IngressOption {
	return func(i *Ingress) { i.router = router }
}

// WithIngressMetrics replaces where the ingress reports to.
func WithIngressMetrics(metrics Metrics) IngressOption {
	return func(i *Ingress) { i.metrics = metrics }
}

// WithRegistry replaces where connected workers are kept.
func WithRegistry(registry Registry) IngressOption {
	return func(i *Ingress) { i.registry = registry }
}

// NewIngress builds an ingress. The authenticator is required: a tunnel that
// takes anything is a way into every worker behind it.
func NewIngress(config Config, auth Authenticator, logger *slog.Logger, options ...IngressOption) (*Ingress, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if auth == nil {
		return nil, errors.New("tunnel: an ingress needs an authenticator")
	}

	i := &Ingress{
		config:   config,
		registry: NewRegistry(),
		auth:     auth,
		router:   LeastLoaded(),
		metrics:  discardMetrics{},
		logger:   logger,
		closed:   make(chan struct{}),
		sessions: make(map[*Session]struct{}),
	}

	for _, option := range options {
		option(i)
	}

	return i, nil
}

// Registry is what the ingress knows about the workers, for reporting on and
// for routing.
func (i *Ingress) Registry() Registry { return i.registry }

// Workers reports every connected worker.
func (i *Ingress) Workers() []WorkerState { return i.registry.Workers() }

// Serve takes connections from listener until ctx is done or Close is called.
//
// Each connection is registered on a goroutine of its own, so one worker taking
// its time over the handshake does not hold up the others.
func (i *Ingress) Serve(ctx context.Context, listener net.Listener) error {
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		select {
		case <-ctx.Done():
		case <-i.closed:
		case <-stop:
		}

		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || i.isClosed() {
				return nil
			}

			return err
		}

		if !i.starting() {
			conn.Close()

			return nil
		}

		go func() {
			defer i.wait.Done()

			i.register(ctx, conn)
		}()
	}
}

// register takes a worker's hello, decides whether to keep the connection, and
// turns it into a session.
func (i *Ingress) register(ctx context.Context, conn net.Conn) {
	deadline := time.Now().Add(i.config.HandshakeTimeout)
	reader := bufio.NewReaderSize(conn, maxFrameBytes)

	var hello registration
	if err := readFrame(conn, reader, &hello, deadline); err != nil {
		i.metrics.AuthenticationFailed("", "handshake")
		i.logger.WarnContext(ctx, "a connection did not register", "error", err)
		conn.Close()

		return
	}

	identity, err := i.accept(ctx, conn, reader, &hello)
	if err != nil {
		i.metrics.AuthenticationFailed(hello.Worker, err.Error())
		i.logger.WarnContext(ctx, "a connection was refused", "worker", hello.Worker, "error", err)

		_ = writeFrame(conn, registered{Error: err.Error()}, deadline)
		conn.Close()

		return
	}

	id := newID()

	if err := writeFrame(conn, registered{OK: true, Session: id}, deadline); err != nil {
		conn.Close()

		return
	}

	// the ingress opens streams, so it is smux's client even though the worker
	// is the one that dialled. Getting this the other way round makes both ends
	// mint stream ids the other does not expect.
	muxSession, err := smux.Client(conn, i.config.smux())
	if err != nil {
		i.logger.WarnContext(ctx, "a session could not be started", "worker", identity.Worker, "error", err)
		conn.Close()

		return
	}

	session := newSession(id, identity.Worker, muxSession, i.streamsPerSession(identity))

	if err := i.registry.Add(session); err != nil {
		i.logger.WarnContext(ctx, "a session could not be registered", "worker", identity.Worker, "error", err)
		muxSession.Close()

		return
	}

	i.track(session)
	i.metrics.SessionOpened(identity.Worker, id)
	i.logger.InfoContext(ctx, "a worker connected", "worker", identity.Worker, "session", id)

	i.watch(ctx, session)
}

// accept settles whether a connection may stay, and as what.
func (i *Ingress) accept(ctx context.Context, conn net.Conn, reader *bufio.Reader, hello *registration) (Identity, error) {
	if hello.Version != ProtocolVersion {
		return Identity{}, ErrUnsupportedVersion
	}

	// smux takes the connection over from here, so anything already read past
	// the newline would be lost — and a peer that sent it is not one of ours.
	if reader.Buffered() > 0 {
		return Identity{}, errors.Join(ErrProtocol, errors.New("spoke before it was registered"))
	}

	identity, err := i.auth.Authenticate(ctx, conn, hello.Worker, hello.Token)
	if err != nil {
		return Identity{}, err
	}

	if len(identity.Worker) == 0 {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("authenticated as nobody"))
	}

	if limit := identity.MaxSessions; limit > 0 {
		if sessions, err := i.registry.Sessions(identity.Worker); err == nil && len(sessions) >= limit {
			return Identity{}, errors.Join(ErrUnauthorized, errors.New("this worker holds as many connections as it may"))
		}
	}

	return identity, nil
}

// streamsPerSession is what one of this worker's sessions may carry.
func (i *Ingress) streamsPerSession(identity Identity) int {
	if identity.MaxStreams > 0 && identity.MaxStreams < i.config.MaxStreamsPerSession {
		return identity.MaxStreams
	}

	return i.config.MaxStreamsPerSession
}

// watch holds a session until it ends, and takes it out when it does. It is
// what makes a worker's existence follow its connections rather than anything
// it has to say.
func (i *Ingress) watch(ctx context.Context, session *Session) {
	select {
	case <-session.Done():
	case <-i.closed:
		session.Close()
	case <-ctx.Done():
		session.Close()
	}

	i.registry.Remove(session)
	i.untrack(session)

	sent, received := session.Traffic()
	i.metrics.SessionClosed(session.Worker(), session.ID(), "closed")
	i.logger.InfoContext(ctx, "a worker's connection ended",
		"worker", session.Worker(),
		"session", session.ID(),
		"sent", sent,
		"received", received,
	)
}

// Dial opens a stream to a named worker and connects it to target.
//
// What comes back behaves as a TCP connection to that target: reading takes
// what it sends, writing reaches it, and closing the writing half tells it that
// nothing more is coming.
func (i *Ingress) Dial(ctx context.Context, worker string, target Target) (net.Conn, error) {
	if !target.Valid() {
		return nil, errors.Join(ErrProtocol, errors.New("a stream has to name a target"))
	}

	deadline := time.Now().Add(i.config.CapacityWait)

	for {
		session, err := i.reserve(worker)
		switch {
		case err == nil:
			stream, err := session.open(ctx, target, i.config.HandshakeTimeout)
			if err != nil {
				session.release()
				i.metrics.StreamFailed(worker, target.String(), "open")

				return nil, err
			}

			i.metrics.StreamOpened(worker, session.ID(), target.String())

			return &accountedConn{Conn: stream, session: session, metrics: i.metrics, target: target}, nil

		case errors.Is(err, ErrAtCapacity):
			// the worker grows its own pool; the ingress cannot make room, only
			// wait for it to be made.
			if !time.Now().Before(deadline) {
				i.metrics.StreamFailed(worker, target.String(), "capacity")

				return nil, err
			}

			select {
			case <-time.After(capacityPollInterval):
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-i.closed:
				return nil, net.ErrClosed
			}

		default:
			i.metrics.StreamFailed(worker, target.String(), "unavailable")

			return nil, err
		}
	}
}

// Route picks a worker and opens a stream to it. The connection it returns
// stays bound to that worker for as long as it lives.
func (i *Ingress) Route(ctx context.Context, target Target) (net.Conn, error) {
	worker, err := i.router.Pick(i.registry.Workers())
	if err != nil {
		i.metrics.StreamFailed("", target.String(), "no worker")

		return nil, err
	}

	return i.Dial(ctx, worker, target)
}

// capacityPollInterval is how often Dial looks again while a worker is full.
const capacityPollInterval = 50 * time.Millisecond

// reserve picks one of a worker's sessions and takes a place on it.
//
// The session with the most room is chosen, which spreads streams evenly and
// keeps the failure of any one connection from taking a disproportionate share
// of them with it.
func (i *Ingress) reserve(worker string) (*Session, error) {
	sessions, err := i.registry.Sessions(worker)
	if err != nil {
		return nil, err
	}

	usable := make([]*Session, 0, len(sessions))
	for _, session := range sessions {
		if !session.Closed() {
			usable = append(usable, session)
		}
	}

	if len(usable) == 0 {
		return nil, ErrNoSuchWorker
	}

	slices.SortFunc(usable, func(a *Session, b *Session) int {
		return b.Free() - a.Free()
	})

	for _, session := range usable {
		if session.reserve() {
			return session, nil
		}
	}

	return nil, ErrAtCapacity
}

// starting counts one more thing this ingress has running, unless it is on its
// way out. Counting up and waiting are ordered by the same lock, which is what
// a WaitGroup requires of anything that does both.
func (i *Ingress) starting() bool {
	i.lock.Lock()
	defer i.lock.Unlock()

	if i.closing {
		return false
	}

	i.wait.Add(1)

	return true
}

func (i *Ingress) track(session *Session) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.sessions[session] = struct{}{}
}

func (i *Ingress) untrack(session *Session) {
	i.lock.Lock()
	defer i.lock.Unlock()

	delete(i.sessions, session)
}

func (i *Ingress) isClosed() bool {
	select {
	case <-i.closed:
		return true
	default:
		return false
	}
}

// Close ends every session and waits for the goroutines holding them to finish,
// so that a closed ingress leaves nothing running.
func (i *Ingress) Close() error {
	i.closeOnce.Do(func() {
		close(i.closed)

		i.lock.Lock()
		i.closing = true
		sessions := make([]*Session, 0, len(i.sessions))
		for session := range i.sessions {
			sessions = append(sessions, session)
		}
		i.lock.Unlock()

		for _, session := range sessions {
			session.Close()
		}
	})

	i.wait.Wait()

	return nil
}

// accountedConn is a stream that gives its place on the session back when it is
// closed, and says how much went through it.
type accountedConn struct {
	net.Conn

	session *Session
	metrics Metrics
	target  Target

	closeOnce sync.Once
}

var (
	_ net.Conn   = &accountedConn{}
	_ halfCloser = &accountedConn{}
)

func (c *accountedConn) CloseWrite() error {
	if closer, ok := c.Conn.(halfCloser); ok {
		return closer.CloseWrite()
	}

	return nil
}

func (c *accountedConn) Close() error {
	c.closeOnce.Do(func() {
		c.session.release()
		c.metrics.StreamClosed(c.session.Worker(), c.session.ID(), c.target.String(), 0, 0)
	})

	return c.Conn.Close()
}

func newID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().Format("20060102150405.000000")
	}

	return hex.EncodeToString(raw[:])
}
