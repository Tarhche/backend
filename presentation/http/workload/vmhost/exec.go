package vmhost

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

const (
	// writeWait bounds one frame's write. A client reads its frames as they
	// come whatever its streams' readers do, so one that has not taken a
	// frame in this long is not there any more.
	writeWait = time.Minute

	// resizeWait bounds telling a command its terminal's new size.
	resizeWait = 5 * time.Second

	// settleWait bounds how long a session that is over waits for what it
	// started to notice. The engine's contract is that closing a command lets
	// go of its streams; one that does not is logged rather than waited on.
	settleWait = 10 * time.Second
)

// errSessionOver is what is left of a session's streams once it is over.
var errSessionOver = errors.New("the exec session is over")

// exec runs a command in a VM and carries it over the request's connection,
// upgraded to wire.ExecProtocol.
//
// The command is started before the connection is upgraded, so a VM that is
// not there or not running is answered as any other request is, with a status
// and an error, and only a command that runs gets a session. The session lasts
// as long as the connection: a client that hangs up ends the command.
func (s *Server) exec(rw http.ResponseWriter, r *http.Request) {
	if !upgrading(r) {
		s.fail(rw, r, fmt.Errorf("%w: an exec upgrades its connection to %s", wire.ErrInvalid, wire.ExecProtocol))

		return
	}

	var options wire.ExecOptions
	if err := decode(r, &options); err != nil {
		s.fail(rw, r, err)

		return
	}

	if len(options.Command) == 0 {
		s.fail(rw, r, fmt.Errorf("%w: an exec runs a command", wire.ErrInvalid))

		return
	}

	if s.isClosed() {
		s.fail(rw, r, fmt.Errorf("%w: it is shutting down", wire.ErrUnavailable))

		return
	}

	// the session's own context, which it ends: the request's ends with the
	// handler, which is about to become the session.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))

	command, err := s.engine.Exec(ctx, r.PathValue("id"), options.ToVM())
	if err != nil {
		cancel()
		s.fail(rw, r, err)

		return
	}

	conn, buffered, err := http.NewResponseController(rw).Hijack()
	if err != nil {
		_ = command.Close()
		cancel()
		s.fail(rw, r, fmt.Errorf("the connection cannot be upgraded: %w", err))

		return
	}

	session := newSession(ctx, cancel, conn, buffered.Reader, command, s.logger)

	if !s.track(session) {
		session.stop()

		return
	}
	defer s.untrack(session)

	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	if _, err := io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: "+wire.ExecProtocol+"\r\n\r\n"); err != nil {
		session.stop()

		return
	}

	session.run()
}

func (s *Server) isClosed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}

// upgrading reports whether a request asks for its connection to be upgraded
// to the exec protocol.
func upgrading(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), wire.ExecProtocol) {
		return false
	}

	for _, value := range r.Header.Values("Connection") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}

	return false
}

// session is one command carried over one upgraded connection.
type session struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   net.Conn
	frames *wire.FrameReader
	logger *slog.Logger

	command vm.ExecSession

	// writing keeps a frame whole while several goroutines write them.
	writing sync.Mutex

	// stdin is what the client sent and the command has not read yet;
	// reading it gives the client its room back.
	stdin *wire.Inbound

	// stdout and stderr are how much more of each the client has room for.
	stdout *wire.Credit
	stderr *wire.Credit

	// work is everything run started.
	work     sync.WaitGroup
	stopping sync.Once
}

func newSession(ctx context.Context, cancel context.CancelFunc, conn net.Conn, buffered *bufio.Reader, command vm.ExecSession, logger *slog.Logger) *session {
	s := &session{
		ctx:     ctx,
		cancel:  cancel,
		conn:    conn,
		frames:  wire.NewFrameReader(buffered),
		logger:  logger,
		command: command,
		stdout:  wire.NewCredit(),
		stderr:  wire.NewCredit(),
	}

	s.stdin = wire.NewInbound(func(n int) {
		_ = s.send(wire.FrameWindow, wire.WindowPayload(wire.FrameStdin, n))
	})

	return s
}

// run carries the command until either side is done with it: the command,
// once it has ended and said all it had to, or the client, by closing it or
// by hanging up.
func (s *session) run() {
	var said sync.WaitGroup

	said.Go(func() { s.carry(wire.FrameStdout, s.command.Stdout(), s.stdout) })
	said.Go(func() { s.carry(wire.FrameStderr, s.command.Stderr(), s.stderr) })

	exited := make(chan struct{})

	s.work.Go(s.feed)
	s.work.Go(func() {
		defer close(exited)

		exitCode, err := s.command.Wait(s.ctx)
		_ = s.send(wire.FrameExit, wire.ExitPayload(exitCode, err))
	})

	// a command that has ended and said everything has nothing left to
	// carry: hanging up is what ends the loop below.
	s.work.Go(func() {
		said.Wait()
		<-exited

		_ = s.conn.Close()
	})

	s.serve()
	s.stop()

	s.settle(&said)
}

// serve reads what the client sends until it closes the session, hangs up,
// or sends what this protocol does not have.
func (s *session) serve() {
	for {
		t, payload, err := s.frames.Next()
		if err != nil {
			return
		}

		if err := s.receive(t, payload); err != nil {
			if !errors.Is(err, errClosedByClient) {
				s.logger.WarnContext(s.ctx, "an exec session was ended for what its client sent", "error", err)
			}

			return
		}
	}
}

// errClosedByClient is a client that closed its session.
var errClosedByClient = errors.New("closed by its client")

func (s *session) receive(t wire.FrameType, payload []byte) error {
	switch t {
	case wire.FrameStdin:
		if len(payload) == 0 {
			s.stdin.End(io.EOF)

			return nil
		}

		return s.stdin.Push(payload)

	case wire.FrameResize:
		rows, cols, err := wire.ParseResize(payload)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(s.ctx, resizeWait)
		defer cancel()

		// a terminal that could not be resized is still a terminal: the
		// command goes on at the size it had.
		_ = s.command.Resize(ctx, rows, cols)

		return nil

	case wire.FrameWindow:
		stream, increment, err := wire.ParseWindow(payload)
		if err != nil {
			return err
		}

		switch stream {
		case wire.FrameStdout:
			return s.stdout.Grant(increment)
		case wire.FrameStderr:
			return s.stderr.Grant(increment)
		default:
			return fmt.Errorf("%w: room for %s, which the client is not sent", wire.ErrProtocol, stream)
		}

	case wire.FrameClose:
		return errClosedByClient

	default:
		return fmt.Errorf("%w: a %s frame from the client", wire.ErrProtocol, t)
	}
}

// carry sends what the command says on one stream, as the client makes room
// for it, and the stream's end once there is no more of it.
//
// Room is waited for before the command's stream is read, so a client that
// does not read a stream holds up the command on that stream, as a pipe
// nobody reads would, and nothing else.
func (s *session) carry(stream wire.FrameType, from io.Reader, credit *wire.Credit) {
	buffer := make([]byte, wire.MaxChunk)

	for {
		room, err := credit.Take(len(buffer))
		if err != nil {
			return
		}

		n, readErr := from.Read(buffer[:room])

		if unused := room - n; unused > 0 {
			_ = credit.Grant(unused)
		}

		if n > 0 {
			if err := s.send(stream, buffer[:n]); err != nil {
				return
			}
		}

		if readErr != nil {
			// the end of the stream, or the command ending under it: either
			// way there is no more of it.
			_ = s.send(stream, nil)

			return
		}
	}
}

// feed writes what the client sends to the command's input, and closes it
// once the client has said that is all.
func (s *session) feed() {
	stdin := s.command.Stdin()
	buffer := make([]byte, wire.MaxChunk)
	closed := false

	for {
		n, err := s.stdin.Read(buffer)

		if n > 0 && !closed {
			if _, err := stdin.Write(buffer[:n]); err != nil {
				// the command's input has closed. The client is told, so
				// what it writes from now on is refused rather than lost;
				// what is already on its way is read and dropped, so the
				// client is never left waiting for room.
				closed = true
				_ = s.send(wire.FrameStdin, nil)
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) && !closed {
				_ = stdin.Close()
			}

			return
		}
	}
}

func (s *session) send(t wire.FrameType, payload []byte) error {
	s.writing.Lock()
	defer s.writing.Unlock()

	_ = s.conn.SetWriteDeadline(time.Now().Add(writeWait))

	return wire.WriteFrame(s.conn, t, payload)
}

// stop ends the session: the connection, and then the command, if it has not
// ended, and whoever waits on either. Hanging up comes first, so a client is
// never told a command ended by its vmhost exited as though it ended on its
// own: what it is told is that its session ended.
func (s *session) stop() {
	s.stopping.Do(func() {
		_ = s.conn.Close()

		_ = s.command.Close()
		s.cancel()

		s.stdin.Fail(errSessionOver)
		s.stdout.Fail(errSessionOver)
		s.stderr.Fail(errSessionOver)
	})
}

// settle waits for what run started to notice the session is over.
func (s *session) settle(said *sync.WaitGroup) {
	done := make(chan struct{})

	go func() {
		said.Wait()
		s.work.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(settleWait):
		s.logger.WarnContext(s.ctx, "an exec session's command did not let go of its streams once closed")
	}
}
