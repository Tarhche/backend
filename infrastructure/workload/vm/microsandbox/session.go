//go:build microsandbox

package microsandbox

import (
	"cmp"
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errSessionClosed is what is left of a session's streams once it is closed.
var errSessionClosed = errors.New("the exec session is closed")

// sessionCloseWait bounds how long closing a session waits for its command to
// be gone.
const sessionCloseWait = 5 * time.Second

// Exec runs a command inside a running instance.
//
// Its input is always a pipe, which is what a terminal, a compose file or a
// connection to dockerd is written through. The command runs with the
// instance's environment, given again every time since a restore drops it
// (#1676), and in the instance's working directory unless it names its own.
// An instance with a main process lets nothing in before that process is
// started, so that the main process is the first session of every boot.
func (e *engine) Exec(ctx context.Context, id string, options vm.ExecOptions) (vm.ExecSession, error) {
	i, err := e.get(id)
	if err != nil {
		return nil, err
	}

	if len(options.Command) == 0 || len(options.Command[0]) == 0 {
		return nil, errors.New("an exec runs a command, and none was given")
	}

	r := i.current()

	if r.Spec.HasMainProcess() {
		if err := i.acquire(ctx); err != nil {
			return nil, err
		}
		i.release()
	}

	sb, err := e.live(ctx, i)
	if err != nil {
		return nil, err
	}

	execOptions := []msb.ExecOption{msb.WithExecStdinPipe()}

	if options.TTY {
		execOptions = append(execOptions, msb.WithExecTTY(true))
	}

	if env := execEnv(r.Spec.Env, options); env != nil {
		execOptions = append(execOptions, msb.WithExecEnv(env))
	}

	if dir := cmp.Or(options.WorkingDir, r.Spec.WorkingDir); len(dir) > 0 {
		execOptions = append(execOptions, msb.WithExecCwd(dir))
	}

	handle, err := sb.ExecStream(ctx, options.Command[0], options.Command[1:], execOptions...)
	if err != nil {
		// a handle into a sandbox that went down and came back meanwhile
		// leads nowhere; the next exec connects afresh.
		e.forget(i)

		return nil, err
	}

	s := newSession(handle, options)

	i.mu.Lock()
	i.sessions[s] = struct{}{}
	i.mu.Unlock()

	go s.pump(func() {
		i.mu.Lock()
		delete(i.sessions, s)
		i.mu.Unlock()
	})

	return s, nil
}

// session is a command exec'd into an instance.
//
// One goroutine reads what microsandbox says of the command, since its events
// are read one at a time, and hands its output to pipes: what the command
// writes waits until it is read, as it would on a terminal nobody looks at.
type session struct {
	handle *msb.ExecHandle

	tty  bool
	rows uint
	cols uint

	stdin *sink

	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter

	stderrReader io.Reader
	stderrWriter *io.PipeWriter

	done chan struct{}

	lock     sync.Mutex
	exitCode int
	closed   bool

	closing sync.Once
}

var _ vm.ExecSession = &session{}

func newSession(handle *msb.ExecHandle, options vm.ExecOptions) *session {
	s := &session{
		handle:   handle,
		tty:      options.TTY,
		rows:     options.Rows,
		cols:     options.Cols,
		stdin:    &sink{sink: handle.TakeStdin()},
		done:     make(chan struct{}),
		exitCode: -1,
	}

	s.stdoutReader, s.stdoutWriter = io.Pipe()

	// under a TTY everything the command says is on its output, and its errors
	// are an empty stream.
	if options.TTY {
		s.stderrReader = strings.NewReader("")
	} else {
		reader, writer := io.Pipe()
		s.stderrReader, s.stderrWriter = reader, writer
	}

	return s
}

// pump hands what the command says to whoever reads it, until it is over, and
// then calls released.
func (s *session) pump(released func()) {
	defer released()
	defer close(s.done)

	var stderr io.Writer = s.stdoutWriter
	if s.stderrWriter != nil {
		stderr = s.stderrWriter
	}

	for {
		event, err := s.handle.Recv(context.Background())
		if err != nil || event.Kind == msb.ExecEventDone {
			break
		}

		switch event.Kind {
		case msb.ExecEventStarted:
			// microsandbox starts every terminal at 24 by 80, so the size
			// the command was asked with is given once it has started.
			if s.tty && s.rows > 0 && s.cols > 0 {
				_ = s.Resize(context.Background(), s.rows, s.cols)
			}
		case msb.ExecEventStdout:
			_, _ = s.stdoutWriter.Write(event.Data)
		case msb.ExecEventStderr:
			_, _ = stderr.Write(event.Data)
		case msb.ExecEventExited:
			s.lock.Lock()
			s.exitCode = event.ExitCode
			s.lock.Unlock()
		case msb.ExecEventFailed:
			// the command never started: a shell would say why, and exit
			// with what says so.
			code, message := 126, "the command could not be started"
			if event.Failure != nil {
				code, message = spawnExitCode(event.Failure.Kind), event.Failure.Message
			}

			s.lock.Lock()
			s.exitCode = code
			s.lock.Unlock()

			_, _ = io.WriteString(stderr, message+"\n")
		}
	}

	// the command is over: its output ends, and what is still written to its
	// input has nothing to read it.
	_ = s.stdoutWriter.Close()
	if s.stderrWriter != nil {
		_ = s.stderrWriter.Close()
	}

	_ = s.stdin.Close()
	_ = s.handle.Close()
}

func (s *session) Stdin() io.WriteCloser {
	return s.stdin
}

func (s *session) Stdout() io.Reader {
	return s.stdoutReader
}

func (s *session) Stderr() io.Reader {
	return s.stderrReader
}

// Resize tells the command's terminal how big it now is.
func (s *session) Resize(ctx context.Context, rows uint, cols uint) error {
	s.lock.Lock()
	closed := s.closed
	s.lock.Unlock()

	if closed {
		return errSessionClosed
	}

	return s.handle.Resize(ctx, uint16(min(rows, math.MaxUint16)), uint16(min(cols, math.MaxUint16)))
}

// Wait waits for the command to end and says how it did. A command killed by
// a signal says -1, as microsandbox does.
func (s *session) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		s.lock.Lock()
		defer s.lock.Unlock()

		return s.exitCode, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Close ends the command, if it is still running, and lets go of whoever is
// waiting on its streams.
func (s *session) Close() error {
	s.closing.Do(func() {
		s.lock.Lock()
		s.closed = true
		s.lock.Unlock()

		_ = s.stdin.Close()

		select {
		case <-s.done:
		default:
			ctx, cancel := context.WithTimeout(context.Background(), sessionCloseWait)
			_ = s.handle.Kill(ctx)
			cancel()
		}

		_ = s.stdoutReader.CloseWithError(errSessionClosed)
		if reader, ok := s.stderrReader.(*io.PipeReader); ok {
			_ = reader.CloseWithError(errSessionClosed)
		}

		select {
		case <-s.done:
		case <-time.After(sessionCloseWait):
		}
	})

	return nil
}

// sink is a command's input. Closing it is the end of the input, once.
type sink struct {
	sink *msb.ExecSink

	lock   sync.Mutex
	closed bool
}

func (s *sink) Write(p []byte) (int, error) {
	s.lock.Lock()
	closed := s.closed
	s.lock.Unlock()

	if closed || s.sink == nil {
		return 0, io.ErrClosedPipe
	}

	return s.sink.Write(p)
}

func (s *sink) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed || s.sink == nil {
		return nil
	}

	s.closed = true

	return s.sink.Close()
}
