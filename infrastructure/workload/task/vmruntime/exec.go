package vmruntime

import (
	"context"
	"io"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// execSession is a command running inside a run's VM, as a task's terminal
// reads and writes it: one stream of output, which under a terminal is all
// there is and otherwise is its output and its errors as they come.
type execSession struct {
	session vm.ExecSession
	output  io.Reader
}

var _ task.ExecSession = &execSession{}

func newExecSession(session vm.ExecSession, tty bool) *execSession {
	if tty {
		return &execSession{session: session, output: session.Stdout()}
	}

	return &execSession{session: session, output: merged(session.Stdout(), session.Stderr())}
}

func (s *execSession) Read(p []byte) (int, error) {
	return s.output.Read(p)
}

func (s *execSession) Write(p []byte) (int, error) {
	return s.session.Stdin().Write(p)
}

func (s *execSession) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.session.Resize(ctx, rows, cols)
}

// Close ends the command: an engine's command ends with its session, unlike
// docker's, which carries on when nobody is attached.
func (s *execSession) Close() error {
	return s.session.Close()
}

// End ends the command, and everything it started, which closing it already
// does.
func (s *execSession) End(context.Context) error {
	return s.session.Close()
}

// merged is two streams read as one, in the order what they carry arrives.
func merged(first io.Reader, second io.Reader) io.Reader {
	reader, writer := io.Pipe()

	var (
		writing sync.Mutex
		copying sync.WaitGroup
	)

	for _, stream := range []io.Reader{first, second} {
		copying.Go(func() {
			buffer := make([]byte, 4<<10)

			for {
				n, err := stream.Read(buffer)
				if n > 0 {
					writing.Lock()
					_, writeErr := writer.Write(buffer[:n])
					writing.Unlock()

					if writeErr != nil {
						return
					}
				}

				if err != nil {
					return
				}
			}
		})
	}

	go func() {
		copying.Wait()
		_ = writer.Close()
	}()

	return reader
}
