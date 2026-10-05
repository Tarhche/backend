package vmhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

const (
	// writeWait bounds one frame's write. The vmhost reads its frames as they
	// come, so one that has not taken a frame in this long is not there.
	writeWait = time.Minute

	// closeWait bounds telling the vmhost a session is closed, which is a
	// courtesy: hanging up ends the command as surely.
	closeWait = time.Second
)

// errSessionClosed is what is left of a session's streams once it is closed.
var errSessionClosed = errors.New("the exec session is closed")

// Exec runs a command in a VM, as a session carried by a connection of its
// own: the exec request upgrades it, and from then on it carries frames.
//
// ctx bounds opening the session. The session itself lasts until the command
// ends or it is closed, whatever becomes of ctx, as any engine's does: a
// caller that wants it ended with ctx closes it then.
func (c *Client) Exec(ctx context.Context, id string, options vm.ExecOptions) (vm.ExecSession, error) {
	ctx, span := c.start(ctx, "Exec", id)
	defer span.End()

	if len(id) == 0 {
		return nil, notThere(id)
	}

	ctx, cancel, bounded := within(ctx, handshakeTimeout)
	defer cancel()

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, record(span, err)
	}

	s, err := c.upgrade(ctx, conn, id, options)
	if err != nil {
		_ = conn.Close()

		if bounded && errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("%w: the vmhost did not open the session within %s", err, handshakeTimeout)
		}

		return nil, record(span, err)
	}

	return s, nil
}

// upgrade asks the vmhost to run the command and upgrade conn to carry it.
// Giving up on ctx interrupts it; what it hands over is no longer ctx's.
func (c *Client) upgrade(ctx context.Context, conn net.Conn, id string, options vm.ExecOptions) (*session, error) {
	body, err := json.Marshal(wire.NewExecOptions(options))
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPost, base+wire.PathVM(id, wire.ActionExec), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", wire.ExecProtocol)
	c.propagation().Inject(ctx, propagation.HeaderCarrier(request.Header))

	interrupt := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Unix(1, 0))
	})

	response, reader, err := handshake(conn, request)

	if !interrupt() {
		return nil, ctx.Err()
	}

	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	if response.StatusCode != http.StatusSwitchingProtocols {
		defer response.Body.Close()

		return nil, errorOf(response)
	}

	if protocol := response.Header.Get("Upgrade"); !strings.EqualFold(protocol, wire.ExecProtocol) {
		return nil, fmt.Errorf("%w: the connection was upgraded to %q", wire.ErrProtocol, protocol)
	}

	return newSession(conn, reader), nil
}

// handshake sends an upgrade request on conn and reads its answer, keeping
// whatever of the session arrived with it.
func handshake(conn net.Conn, request *http.Request) (*http.Response, *bufio.Reader, error) {
	if err := request.Write(conn); err != nil {
		return nil, nil, fmt.Errorf("the exec request could not be sent: %w", err)
	}

	reader := bufio.NewReader(conn)

	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, nil, fmt.Errorf("the vmhost's answer to the exec request cannot be read: %w", err)
	}

	return response, reader, nil
}

// session is a command running in a VM, carried by its own connection to the
// vmhost.
type session struct {
	conn   net.Conn
	frames *wire.FrameReader

	// writing keeps a frame whole while several goroutines write them.
	writing sync.Mutex

	stdin  *stdin
	stdout *wire.Inbound
	stderr *wire.Inbound

	exited   chan struct{}
	exiting  sync.Once
	exitCode int
	exitErr  error

	closed  chan struct{}
	closing sync.Once

	// received is closed once nothing more is read from the connection.
	received chan struct{}
}

var _ vm.ExecSession = &session{}

func newSession(conn net.Conn, reader *bufio.Reader) *session {
	s := &session{
		conn:     conn,
		frames:   wire.NewFrameReader(reader),
		exited:   make(chan struct{}),
		closed:   make(chan struct{}),
		received: make(chan struct{}),
	}

	s.stdin = &stdin{session: s, credit: wire.NewCredit()}
	s.stdout = wire.NewInbound(s.room(wire.FrameStdout))
	s.stderr = wire.NewInbound(s.room(wire.FrameStderr))

	go s.receive()

	return s
}

func (s *session) Stdin() io.WriteCloser {
	return s.stdin
}

func (s *session) Stdout() io.Reader {
	return s.stdout
}

func (s *session) Stderr() io.Reader {
	return s.stderr
}

// Resize tells the command's terminal how big it now is.
func (s *session) Resize(ctx context.Context, rows uint, cols uint) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.send(wire.FrameResize, wire.ResizePayload(rows, cols))
}

// Wait waits for the command to end and says how it did.
func (s *session) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.exited:
		return s.exitCode, s.exitErr
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Close ends the command. The vmhost is told so and then hung up on, which
// ends the command as surely, so a vmhost too busy to read the one hears the
// other.
func (s *session) Close() error {
	s.closing.Do(func() {
		close(s.closed)

		if s.writing.TryLock() {
			_ = s.conn.SetWriteDeadline(time.Now().Add(closeWait))
			_ = wire.WriteFrame(s.conn, wire.FrameClose, nil)
			s.writing.Unlock()
		}

		_ = s.conn.Close()

		s.stdin.credit.Fail(errSessionClosed)
		s.stdout.Fail(errSessionClosed)
		s.stderr.Fail(errSessionClosed)
		s.exit(0, errSessionClosed)
	})

	<-s.received

	return nil
}

// receive reads what the vmhost sends until the connection ends, and then
// ends whatever has not ended yet with it.
func (s *session) receive() {
	defer close(s.received)

	err := s.read()

	select {
	case <-s.closed:
		err = errSessionClosed
	default:
		err = fmt.Errorf("%w: the exec session's connection to the vmhost ended: %v", io.ErrUnexpectedEOF, err)
	}

	// a session the vmhost finished has every stream ended and its exit
	// said already, so none of this changes anything.
	s.stdout.End(err)
	s.stderr.End(err)
	s.stdin.credit.Fail(io.ErrClosedPipe)
	s.exit(0, err)
}

func (s *session) read() error {
	for {
		t, payload, err := s.frames.Next()
		if err != nil {
			return err
		}

		switch t {
		case wire.FrameStdout:
			if err := s.arrived(s.stdout, payload); err != nil {
				return err
			}

		case wire.FrameStderr:
			if err := s.arrived(s.stderr, payload); err != nil {
				return err
			}

		case wire.FrameExit:
			exitCode, reason, err := wire.ParseExit(payload)
			if err != nil {
				return err
			}

			var waitErr error
			if len(reason) > 0 {
				waitErr = fmt.Errorf("the vmhost could not wait for the command: %s", reason)
			}

			s.exit(exitCode, waitErr)

			// the command has ended, and its input with it.
			s.stdin.credit.Fail(io.ErrClosedPipe)

		case wire.FrameWindow:
			stream, increment, err := wire.ParseWindow(payload)
			if err != nil {
				return err
			}

			if stream != wire.FrameStdin {
				return fmt.Errorf("%w: room for %s, which the vmhost is not sent", wire.ErrProtocol, stream)
			}

			if err := s.stdin.credit.Grant(increment); err != nil {
				return err
			}

		case wire.FrameStdin:
			// the command's input has closed: what is written to it from now
			// on is refused, as a closed pipe refuses it.
			s.stdin.credit.Fail(io.ErrClosedPipe)

		default:
			return fmt.Errorf("%w: a %s frame from the vmhost", wire.ErrProtocol, t)
		}
	}
}

// arrived is what arrived of a stream: more of it, or, empty, its end.
func (s *session) arrived(stream *wire.Inbound, payload []byte) error {
	if len(payload) == 0 {
		stream.End(io.EOF)

		return nil
	}

	return stream.Push(payload)
}

func (s *session) exit(exitCode int, err error) {
	s.exiting.Do(func() {
		s.exitCode, s.exitErr = exitCode, err
		close(s.exited)
	})
}

// room gives the vmhost room for more of stream.
func (s *session) room(stream wire.FrameType) func(int) {
	return func(n int) {
		_ = s.send(wire.FrameWindow, wire.WindowPayload(stream, n))
	}
}

func (s *session) send(t wire.FrameType, payload []byte) error {
	s.writing.Lock()
	defer s.writing.Unlock()

	select {
	case <-s.closed:
		return errSessionClosed
	default:
	}

	_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))

	if err := wire.WriteFrame(s.conn, t, payload); err != nil {
		return fmt.Errorf("the exec session's connection to the vmhost: %w", err)
	}

	return nil
}

// stdin is a command's input. A write waits for the vmhost to have room for
// it, as a write to a full pipe waits for its reader.
type stdin struct {
	session *session
	credit  *wire.Credit

	// writing keeps one write's chunks together, in order.
	writing sync.Mutex
	ended   atomic.Bool
}

func (w *stdin) Write(p []byte) (int, error) {
	w.writing.Lock()
	defer w.writing.Unlock()

	if w.ended.Load() {
		return 0, io.ErrClosedPipe
	}

	written := 0

	for len(p) > 0 {
		n, err := w.credit.Take(min(len(p), wire.MaxChunk))
		if err != nil {
			return written, err
		}

		if err := w.session.send(wire.FrameStdin, p[:n]); err != nil {
			return written, err
		}

		written += n
		p = p[n:]
	}

	return written, nil
}

// Close ends the command's input. A write waiting for room meanwhile is
// refused, as a write to a pipe whose writer closed it is. The end is sent
// when it can be; a session that has ended has a command with no input left
// to end, so closing never fails, as closing a pipe does not.
func (w *stdin) Close() error {
	if w.ended.Swap(true) {
		return nil
	}

	w.credit.Fail(io.ErrClosedPipe)
	_ = w.session.send(wire.FrameStdin, nil)

	return nil
}
