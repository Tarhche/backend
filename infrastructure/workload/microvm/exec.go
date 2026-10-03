package microvm

import (
	"context"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// how a command left without anybody attached to it is ended: it is given a
// moment to finish on its own, then asked to stop, and then stopped for good.
// They are the container driver's, so a terminal walked away from is treated
// the same whichever class the task runs in.
const (
	terminationGrace = 5 * time.Second
	killGrace        = 5 * time.Second
)

// execSession is a command running inside a VM alongside its task, carried on
// one connection through vmhost to the VM's agent as guest frames: its input,
// its output, its terminal's size, and its end.
//
// Unlike docker's, a command here can be ended properly: the agent runs it in
// a cgroup of its own, so everything it started is found however it went
// about starting it, and nothing has to be swept for by a mark in the
// environment.
type execSession struct {
	client *Client
	vmID   string
	execID string
	conn   net.Conn

	// what has been read of a frame but not yet handed to a reader, and
	// whether the command has said it ended. Only one goroutine reads.
	pending []byte
	exited  bool

	// writing serialises the frames going in: input and resizes come from
	// different goroutines, and a frame is written whole or not at all.
	writing sync.Mutex

	closing sync.Once
	closed  error
}

var _ task.ExecSession = &execSession{}

// Read takes the command's output, both streams as one, as a terminal shows
// them. It ends with io.EOF once the command has ended.
func (s *execSession) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		if s.exited {
			return 0, io.EOF
		}

		frame, err := guest.ReadFrame(s.conn)
		if err != nil {
			return 0, err
		}

		switch frame.Type {
		case guest.FrameStdout, guest.FrameStderr:
			s.pending = frame.Payload
		case guest.FrameExit:
			s.exited = true
		}
	}

	n := copy(p, s.pending)
	s.pending = s.pending[n:]

	return n, nil
}

// Write feeds the command's input.
func (s *execSession) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	s.writing.Lock()
	defer s.writing.Unlock()

	if err := guest.WriteFrame(s.conn, guest.FrameStdin, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Resize tells the command's terminal how big it now is, so what it draws
// fits the window the client shows it in.
func (s *execSession) Resize(ctx context.Context, rows uint, cols uint) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.writing.Lock()
	defer s.writing.Unlock()

	return guest.WriteFrame(s.conn, guest.FrameResize, guest.ResizePayload(dimension(rows), dimension(cols)))
}

// Close lets go of the connection the command runs on. It is safe to call
// more than once, and from a goroutine other than the one reading: closing the
// connection is what releases a reader parked in Read.
//
// What was running carries on, which is what End is for.
func (s *execSession) Close() error {
	s.closing.Do(func() {
		s.closed = s.conn.Close()
	})

	return s.closed
}

// End stops the command and everything it started, once nobody is attached to
// it any more. The agent gives it its grace before each signal, so this
// returns once the command is gone, or was gone already.
func (s *execSession) End(ctx context.Context) error {
	_, err := s.client.EndExec(ctx, s.vmID, s.execID, guest.EndExec{Grace: terminationGrace, KillGrace: killGrace})

	return err
}

// dimension is one side of a terminal as a frame carries it, which is at most
// what two bytes hold.
func dimension(size uint) uint16 {
	return uint16(min(size, math.MaxUint16))
}
