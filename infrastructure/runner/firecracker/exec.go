package firecracker

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// how a command left without a client is ended: it is given a moment to finish
// on its own, then asked to stop, and then stopped for good.
const (
	terminationGrace = 5 * time.Second
	killGrace        = 5 * time.Second
)

// execSession is a command running inside a machine alongside its task.
type execSession struct {
	stream *guest.ExecStream
	client *guest.Client
}

var _ task.ExecSession = &execSession{}

func (s *execSession) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

func (s *execSession) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

// Resize tells the command's terminal how big it now is.
func (s *execSession) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.stream.Resize(uint16(rows), uint16(cols))
}

// Close lets go of the stream the command runs on. What was running carries
// on: End is what stops it.
func (s *execSession) Close() error {
	return s.stream.Close()
}

// End stops the command and everything it started. Everything it started is
// in a cgroup of its own inside the machine, so nothing it left behind is
// missed, however it went about leaving it.
func (s *execSession) End(ctx context.Context) error {
	_, err := s.client.EndExec(ctx, s.stream.ID, guest.EndExec{Grace: terminationGrace, KillGrace: killGrace})

	return err
}
