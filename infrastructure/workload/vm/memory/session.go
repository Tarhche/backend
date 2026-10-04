package memory

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errSessionClosed is what is left of a session's streams once it is closed.
var errSessionClosed = errors.New("the exec session is closed")

// Session is a command exec'd into an instance of the memory engine. Its
// streams are pipes: what is written to Stdin is what the command reads, and
// what the command writes is what Stdout and Stderr read.
type Session struct {
	tty bool

	stdinReader *io.PipeReader
	stdinWriter *io.PipeWriter

	stdoutReader *io.PipeReader
	stdoutWriter *io.PipeWriter

	stderrReader io.Reader
	stderrWriter *io.PipeWriter

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	lock     sync.Mutex
	exitCode int
	rows     uint
	cols     uint
	closed   bool

	closing sync.Once
}

var _ vm.ExecSession = &Session{}

func newSession(options vm.ExecOptions) *Session {
	s := &Session{
		tty:  options.TTY,
		rows: options.Rows,
		cols: options.Cols,
		done: make(chan struct{}),
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.stdinReader, s.stdinWriter = io.Pipe()
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

// start runs the command, and calls released once it has ended.
func (s *Session) start(run func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) int, released func()) {
	var stderr io.Writer = s.stdoutWriter
	if s.stderrWriter != nil {
		stderr = s.stderrWriter
	}

	go func() {
		defer released()
		defer close(s.done)

		exitCode := run(s.ctx, s.stdinReader, s.stdoutWriter, stderr)

		s.lock.Lock()
		s.exitCode = exitCode
		s.lock.Unlock()

		// the command is over: its output ends, and what is still written to
		// its input has nothing to read it.
		_ = s.stdoutWriter.Close()
		if s.stderrWriter != nil {
			_ = s.stderrWriter.Close()
		}

		_ = s.stdinReader.CloseWithError(io.ErrClosedPipe)
		s.cancel()
	}()
}

func (s *Session) Stdin() io.WriteCloser {
	return s.stdinWriter
}

func (s *Session) Stdout() io.Reader {
	return s.stdoutReader
}

func (s *Session) Stderr() io.Reader {
	return s.stderrReader
}

// Resize is the size the command's terminal is told it now is, which Size
// reads back.
func (s *Session) Resize(ctx context.Context, rows uint, cols uint) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return errSessionClosed
	}

	s.rows, s.cols = rows, cols

	return nil
}

// Size is how big the command's terminal was last said to be.
func (s *Session) Size() (rows uint, cols uint) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.rows, s.cols
}

func (s *Session) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		s.lock.Lock()
		defer s.lock.Unlock()

		return s.exitCode, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Close ends the command, as the engine's contract says closing a session
// does, and lets go of whoever is waiting on its streams.
func (s *Session) Close() error {
	s.closing.Do(func() {
		s.lock.Lock()
		s.closed = true
		s.lock.Unlock()

		s.cancel()

		_ = s.stdinReader.CloseWithError(errSessionClosed)
		_ = s.stdoutReader.CloseWithError(errSessionClosed)

		if reader, ok := s.stderrReader.(*io.PipeReader); ok {
			_ = reader.CloseWithError(errSessionClosed)
		}
	})

	return nil
}
