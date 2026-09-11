package tunnel

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/xtaci/smux"
)

// Dialer opens the transport a worker reaches an ingress over. It is an
// argument rather than a fact so that a test can join the two ends in memory,
// and so that TLS is a choice made outside this file.
type Dialer interface {
	DialContext(ctx context.Context, address string) (net.Conn, error)
}

// DialerFunc adapts a function to a Dialer.
type DialerFunc func(ctx context.Context, address string) (net.Conn, error)

func (f DialerFunc) DialContext(ctx context.Context, address string) (net.Conn, error) {
	return f(ctx, address)
}

// TLSDialer reaches an ingress over TLS, which is what the token is safe to
// cross inside. Giving the configuration a client certificate is all that
// mutual TLS needs from this end.
func TLSDialer(config *tls.Config, timeout time.Duration) Dialer {
	return DialerFunc(func(ctx context.Context, address string) (net.Conn, error) {
		dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: config}

		return dialer.DialContext(ctx, "tcp", address)
	})
}

// Worker is the far end of the tunnel: a pool of connections to each ingress,
// and the thing that connects the streams arriving on them to their targets.
//
// Nothing dials a worker. It holds connections outwards and answers on them, so
// it needs no address, no open port and no way in.
type Worker struct {
	id      string
	token   string
	config  Config
	dialer  Dialer
	targets Targets
	metrics Metrics
	logger  *slog.Logger

	pools []*sessionPool

	closeOnce sync.Once
	closed    chan struct{}
	wait      sync.WaitGroup
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WithToken is what the worker proves itself with.
func WithToken(token string) WorkerOption {
	return func(w *Worker) { w.token = token }
}

// WithWorkerMetrics replaces where the worker reports to.
func WithWorkerMetrics(metrics Metrics) WorkerOption {
	return func(w *Worker) { w.metrics = metrics }
}

// NewWorker builds a worker that keeps a pool at each of the given addresses.
//
// A worker connected to several ingresses is reachable through all of them,
// which is what lets there be more than one: they hold no shared state, so each
// has to have been connected to.
func NewWorker(
	id string,
	addresses []string,
	config Config,
	dialer Dialer,
	targets Targets,
	logger *slog.Logger,
	options ...WorkerOption,
) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	switch {
	case len(id) == 0:
		return nil, errors.New("tunnel: a worker needs a name")
	case len(addresses) == 0:
		return nil, errors.New("tunnel: a worker needs an ingress to connect to")
	case dialer == nil:
		return nil, errors.New("tunnel: a worker needs a dialer")
	case targets == nil:
		return nil, errors.New("tunnel: a worker needs to know what it may connect to")
	}

	w := &Worker{
		id:      id,
		config:  config,
		dialer:  dialer,
		targets: targets,
		metrics: discardMetrics{},
		logger:  logger,
		closed:  make(chan struct{}),
	}

	for _, option := range options {
		option(w)
	}

	for _, address := range addresses {
		w.pools = append(w.pools, newSessionPool(w, address))
	}

	return w, nil
}

// Run keeps every pool at size until ctx is done, and returns once everything
// it started has stopped.
func (w *Worker) Run(ctx context.Context) {
	defer w.Close()

	var running sync.WaitGroup
	for _, pool := range w.pools {
		running.Add(1)

		go func(pool *sessionPool) {
			defer running.Done()

			pool.run(ctx)
		}(pool)
	}

	running.Wait()
	w.wait.Wait()
}

// Sessions is how many connections the worker is holding, across every ingress.
func (w *Worker) Sessions() int {
	total := 0
	for _, pool := range w.pools {
		total += pool.size()
	}

	return total
}

// Streams is how many streams the worker is carrying.
func (w *Worker) Streams() int {
	total := 0
	for _, pool := range w.pools {
		total += pool.streams()
	}

	return total
}

// Close ends every session. Streams on them end the way a TCP connection ends.
func (w *Worker) Close() error {
	w.closeOnce.Do(func() {
		close(w.closed)

		for _, pool := range w.pools {
			pool.closeAll()
		}
	})

	return nil
}

func (w *Worker) isClosed() bool {
	select {
	case <-w.closed:
		return true
	default:
		return false
	}
}

// serve answers the streams arriving on one session until it ends.
func (w *Worker) serve(ctx context.Context, session *smux.Session, id string) {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}

		w.wait.Add(1)

		go func() {
			defer w.wait.Done()

			w.handle(ctx, stream, id)
		}()
	}
}

// handle connects one stream to what it asked for.
func (w *Worker) handle(ctx context.Context, stream *smux.Stream, session string) {
	deadline := time.Now().Add(w.config.HandshakeTimeout)

	var target Target
	leftover, err := readStreamFrame(stream, &target, deadline)
	if err != nil {
		w.logger.WarnContext(ctx, "a stream did not say what it was for", "session", session, "error", err)
		stream.Close()

		return
	}

	address, err := w.targets.Resolve(ctx, target)
	if err != nil {
		w.refuse(ctx, stream, session, target, err, deadline)

		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, w.config.DialTimeout)
	defer cancel()

	dialer := net.Dialer{}
	upstream, err := dialer.DialContext(dialCtx, "tcp", address)
	if err != nil {
		w.metrics.StreamFailed(w.id, target.String(), "target")
		w.refuse(ctx, stream, session, target, err, deadline)

		return
	}

	if err := writeStreamFrame(stream, opened{OK: true}, deadline); err != nil {
		upstream.Close()
		stream.Close()

		return
	}

	if err := stream.SetDeadline(time.Time{}); err != nil {
		upstream.Close()
		stream.Close()

		return
	}

	w.metrics.StreamOpened(w.id, session, target.String())

	// from here neither end knows what it is carrying, and neither needs to.
	sent, received, err := StreamProxy{}.Copy(withPrefix(stream, leftover), upstream)

	w.metrics.StreamClosed(w.id, session, target.String(), sent, received)

	if err != nil {
		w.logger.DebugContext(ctx, "a stream ended badly",
			"session", session,
			"target", target.String(),
			"sent", sent,
			"received", received,
			"error", err,
		)
	}
}

// refuse tells the ingress why a stream is going no further, so that a target
// which could not be reached is reported rather than looking like one that
// accepted and said nothing.
func (w *Worker) refuse(ctx context.Context, stream *smux.Stream, session string, target Target, reason error, deadline time.Time) {
	w.logger.WarnContext(ctx, "a stream could not be connected",
		"session", session,
		"target", target.String(),
		"error", reason,
	)

	_ = writeStreamFrame(stream, opened{Error: reason.Error()}, deadline)
	stream.Close()
}

// sessionPool is a worker's connections to one ingress.
//
// It grows before it is full rather than when it is: the ingress cannot make
// room, only wait for room to be made, so waiting until capacity is reached
// means clients wait too.
type sessionPool struct {
	worker  *Worker
	address string

	wake chan struct{}

	lock     sync.Mutex
	sessions []*pooledSession
	attempt  int
}

type pooledSession struct {
	id      string
	session *smux.Session
	opened  time.Time
	idle    time.Time
}

func newSessionPool(worker *Worker, address string) *sessionPool {
	return &sessionPool{worker: worker, address: address, wake: make(chan struct{}, 1)}
}

func (p *sessionPool) run(ctx context.Context) {
	ticker := time.NewTicker(poolInterval)
	defer ticker.Stop()

	for {
		if err := p.reconcile(ctx); err != nil && ctx.Err() == nil && !p.worker.isClosed() {
			p.worker.logger.WarnContext(ctx, "could not reach an ingress",
				"address", p.address,
				"attempt", p.attempt,
				"error", err,
			)

			select {
			case <-time.After(p.backoff()):
			case <-p.worker.closed:
				return
			case <-ctx.Done():
				return
			}

			continue
		}

		select {
		case <-ticker.C:
		case <-p.wake:
		case <-p.worker.closed:
			return
		case <-ctx.Done():
			return
		}
	}
}

// poolInterval is how often the pool is looked at when nothing has prompted it.
const poolInterval = 1 * time.Second

// reconcile brings the pool to the size the load calls for.
func (p *sessionPool) reconcile(ctx context.Context) error {
	p.prune()

	for {
		if p.worker.isClosed() || ctx.Err() != nil {
			return nil
		}

		size, streams := p.state()

		enough := size >= p.worker.config.MinSessions
		room := float64(streams) < float64(size*p.worker.config.MaxStreamsPerSession)*p.worker.config.GrowThreshold

		if size >= p.worker.config.MaxSessions || (enough && room) {
			break
		}

		if err := p.connect(ctx); err != nil {
			return err
		}
	}

	p.reap()

	return nil
}

// prune forgets the sessions whose connection has gone. Nothing is migrated:
// the streams that were on them have already failed, which is what a TCP
// connection failing looks like to whatever was using it.
func (p *sessionPool) prune() {
	p.lock.Lock()
	defer p.lock.Unlock()

	kept := p.sessions[:0]
	for _, session := range p.sessions {
		if session.session.IsClosed() {
			continue
		}

		kept = append(kept, session)
	}

	p.sessions = kept
}

// reap closes the sessions above the minimum that have been carrying nothing
// for longer than they are allowed to.
func (p *sessionPool) reap() {
	if p.worker.config.IdleSessionTimeout <= 0 {
		return
	}

	now := time.Now()

	p.lock.Lock()
	var stale []*pooledSession
	for _, session := range p.sessions {
		if session.session.NumStreams() > 0 {
			session.idle = now

			continue
		}

		if len(p.sessions)-len(stale) <= p.worker.config.MinSessions {
			break
		}

		if now.Sub(session.idle) > p.worker.config.IdleSessionTimeout {
			stale = append(stale, session)
		}
	}
	p.lock.Unlock()

	for _, session := range stale {
		session.session.Close()
	}
}

// connect opens one connection, registers on it, and starts serving it.
func (p *sessionPool) connect(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, p.worker.config.DialTimeout)
	defer cancel()

	conn, err := p.worker.dialer.DialContext(dialCtx, p.address)
	if err != nil {
		p.failed()

		return err
	}

	id, err := p.registerOn(conn)
	if err != nil {
		conn.Close()
		p.failed()

		return err
	}

	// the ingress opens the streams, so it is smux's client and this is the
	// server, on a connection this end dialled.
	session, err := smux.Server(conn, p.worker.config.smux())
	if err != nil {
		conn.Close()
		p.failed()

		return err
	}

	held := &pooledSession{id: id, session: session, opened: time.Now(), idle: time.Now()}

	p.lock.Lock()
	closed := p.worker.isClosed()
	if !closed {
		p.sessions = append(p.sessions, held)
		p.attempt = 0
	}
	p.lock.Unlock()

	if closed {
		// closed while this one was being opened. Letting it go is what tells
		// the ingress the worker is leaving rather than arriving.
		session.Close()

		return nil
	}

	p.worker.metrics.SessionOpened(p.worker.id, id)
	p.worker.logger.InfoContext(ctx, "connected to an ingress", "address", p.address, "session", id)

	p.worker.wait.Add(1)

	go func() {
		defer p.worker.wait.Done()
		defer p.nudge()

		p.worker.serve(ctx, session, id)

		p.worker.metrics.SessionClosed(p.worker.id, id, "closed")
		p.worker.logger.InfoContext(ctx, "a connection to an ingress ended", "address", p.address, "session", id)
	}()

	return nil
}

// registerOn says which worker this is and waits to be told it may stay.
func (p *sessionPool) registerOn(conn net.Conn) (string, error) {
	deadline := time.Now().Add(p.worker.config.HandshakeTimeout)

	hello := registration{Version: ProtocolVersion, Worker: p.worker.id, Token: p.worker.token}
	if err := writeFrame(conn, hello, deadline); err != nil {
		return "", err
	}

	reader := bufio.NewReaderSize(conn, maxFrameBytes)

	var answer registered
	if err := readFrame(conn, reader, &answer, deadline); err != nil {
		return "", err
	}

	if !answer.OK {
		return "", rejection(answer.Error)
	}

	// smux takes the connection from here, so anything read past the newline
	// would be lost.
	if reader.Buffered() > 0 {
		return "", errors.Join(ErrProtocol, errors.New("the ingress spoke before it was asked to"))
	}

	return answer.Session, nil
}

func (p *sessionPool) failed() {
	p.lock.Lock()
	p.attempt++
	attempt := p.attempt
	p.lock.Unlock()

	p.worker.metrics.Reconnected(p.worker.id, p.address, attempt)
}

// backoff is how long to wait before dialling again: doubling to a ceiling, and
// then a wait drawn from anywhere in that range rather than the range's end.
//
// The jitter is the point. A thousand workers that lost the same ingress would
// otherwise come back in step, and knock it over again the moment it stood up.
func (p *sessionPool) backoff() time.Duration {
	p.lock.Lock()
	attempt := p.attempt
	p.lock.Unlock()

	delay := p.worker.config.ReconnectMinDelay
	for range max(attempt-1, 0) {
		delay *= 2
		if delay >= p.worker.config.ReconnectMaxDelay {
			delay = p.worker.config.ReconnectMaxDelay

			break
		}
	}

	return time.Duration(rand.Int64N(int64(delay)) + 1)
}

func (p *sessionPool) state() (sessions int, streams int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, session := range p.sessions {
		sessions++
		streams += session.session.NumStreams()
	}

	return sessions, streams
}

func (p *sessionPool) size() int {
	sessions, _ := p.state()

	return sessions
}

func (p *sessionPool) streams() int {
	_, streams := p.state()

	return streams
}

func (p *sessionPool) closeAll() {
	p.lock.Lock()
	sessions := make([]*pooledSession, len(p.sessions))
	copy(sessions, p.sessions)
	p.lock.Unlock()

	for _, session := range sessions {
		session.session.Close()
	}
}

// nudge asks the pool to look at itself now rather than at the next tick, which
// is what makes a lost session replaced immediately.
func (p *sessionPool) nudge() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
