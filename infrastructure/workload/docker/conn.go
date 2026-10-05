package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// chunkSize is how much of what dockerd says is read off the exec session
	// at a time.
	chunkSize = 32 << 10

	// exitWait bounds how long a connection whose command ended before it said
	// anything waits to be told why.
	exitWait = 2 * time.Second

	// stderrKept is how much of what the command said on its errors is kept,
	// which is enough for docker's own one-line complaint.
	stderrKept = 1 << 10
)

// conn is a connection to the dockerd of a Docker VM, carried by a command
// running inside the VM.
//
// `docker system dial-stdio` is docker's own way of reaching a daemon through
// something that is not a socket: what it reads on its input it sends to
// dockerd's socket, and what dockerd answers it writes to its output. So a
// write here is the command's input, and a read is its output. Nothing about
// the daemon is exposed on any network, and it works whatever the VM's
// network allows.
//
// It is a net.Conn as the Docker client and net/http need one. Closing it ends
// the command. Deadlines are honoured as a pipe honours them: a read waits for
// its deadline, and a write is refused once its deadline has passed but is not
// interrupted once it has started, since the session's input cannot be.
type conn struct {
	session vm.ExecSession
	vmUUID  string

	stdin   io.WriteCloser
	writing sync.Mutex

	// chunks is what the command said, in order. It is closed once the output
	// ends, and readErr is why, set before it is closed.
	chunks  chan []byte
	readErr error

	reading sync.Mutex
	pending []byte

	// stderr is the last of what the command said on its errors, which is
	// what says why a command that ended at once did.
	stderr     *tailBuffer
	stderrDone chan struct{}

	// reasoning works out once why the command ended, for whichever of
	// reading and writing finds out first.
	reasoning sync.Once
	reasonErr error

	readDeadline  *deadline
	writeDeadline *deadline

	closed  chan struct{}
	closing sync.Once
}

var _ net.Conn = &conn{}

func newConn(session vm.ExecSession, vmUUID string) *conn {
	c := &conn{
		session:       session,
		vmUUID:        vmUUID,
		stdin:         session.Stdin(),
		chunks:        make(chan []byte),
		stderr:        newTailBuffer(stderrKept),
		stderrDone:    make(chan struct{}),
		readDeadline:  newDeadline(),
		writeDeadline: newDeadline(),
		closed:        make(chan struct{}),
	}

	go c.drainStderr()
	go c.pump()

	return c
}

// drainStderr keeps what the command says on its errors. Nothing else reads
// them, and a command whose errors nobody reads may wait for somebody to.
func (c *conn) drainStderr() {
	defer close(c.stderrDone)

	_, _ = io.Copy(c.stderr, c.session.Stderr())
}

// pump carries what the command says to whoever reads the connection.
func (c *conn) pump() {
	buffer := make([]byte, chunkSize)
	stdout := c.session.Stdout()
	said := false

	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			said = true

			select {
			case c.chunks <- bytes.Clone(buffer[:n]):
			case <-c.closed:
				c.end(net.ErrClosed)

				return
			}
		}

		if err == nil {
			continue
		}

		if errors.Is(err, io.EOF) && !said {
			err = c.reason()
		}

		c.end(err)

		return
	}
}

// reason is why the command ended, worked out once.
func (c *conn) reason() error {
	c.reasoning.Do(func() {
		c.reasonErr = c.why()
	})

	return c.reasonErr
}

// end is the end of what the command says, for err.
func (c *conn) end(err error) {
	c.readErr = err
	close(c.chunks)
}

// why is what a command that ended before it said anything ended with.
//
// A shell that has no docker command answers 127, and one that cannot run it
// 126: that is a VM with no docker in it, which waiting will not change.
// Anything else is a daemon that is not answering yet, or not at all, and
// what the command said about it is all there is to go on.
func (c *conn) why() error {
	ctx, cancel := context.WithTimeout(context.Background(), exitWait)
	defer cancel()

	exitCode, err := c.session.Wait(ctx)
	if err != nil {
		return io.EOF
	}

	select {
	case <-c.stderrDone:
	case <-ctx.Done():
	}

	said := strings.TrimSpace(c.stderr.String())

	switch exitCode {
	case 126, 127:
		return fmt.Errorf("%w: it has no docker command to reach its daemon with (%s)", vm.ErrNotDocker, said)
	default:
		return fmt.Errorf("%w: docker system dial-stdio exited with %d: %s", io.ErrUnexpectedEOF, exitCode, said)
	}
}

func (c *conn) Read(p []byte) (int, error) {
	c.reading.Lock()
	defer c.reading.Unlock()

	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}

	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]

		return n, nil
	}

	select {
	case <-c.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	default:
	}

	select {
	case chunk, ok := <-c.chunks:
		if !ok {
			return 0, c.readErr
		}

		n := copy(p, chunk)
		c.pending = chunk[n:]

		return n, nil
	case <-c.closed:
		return 0, net.ErrClosed
	case <-c.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *conn) Write(p []byte) (int, error) {
	c.writing.Lock()
	defer c.writing.Unlock()

	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-c.writeDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	default:
	}

	n, err := c.stdin.Write(p)
	if err == nil {
		return n, nil
	}

	select {
	case <-c.closed:
		return n, net.ErrClosed
	default:
	}

	// the command has ended, which is why its input is closed: what it ended
	// with says more than that, and a request is written before anything is
	// read, so this is where a command that ended at once is found out.
	if reason := c.reason(); !errors.Is(reason, io.EOF) {
		return n, reason
	}

	return n, err
}

// Close ends the command carrying the connection, which is what closes
// dockerd's end of it, and lets go of whoever is reading.
func (c *conn) Close() error {
	err := net.ErrClosed

	c.closing.Do(func() {
		close(c.closed)

		c.readDeadline.stop()
		c.writeDeadline.stop()

		_ = c.stdin.Close()
		err = c.session.Close()
	})

	return err
}

func (c *conn) LocalAddr() net.Addr {
	return execAddr("orchestrator")
}

func (c *conn) RemoteAddr() net.Addr {
	return execAddr("vm/" + c.vmUUID)
}

func (c *conn) SetDeadline(t time.Time) error {
	c.readDeadline.set(t)
	c.writeDeadline.set(t)

	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.set(t)

	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.set(t)

	return nil
}

// execAddr is an end of a connection carried by an exec session, which has no
// network address: it is named by what it is.
type execAddr string

func (a execAddr) Network() string {
	return "exec"
}

func (a execAddr) String() string {
	return string(a)
}

// deadline is a moment after which waiting is given up, as net.Pipe keeps
// one: a channel that is closed when the moment passes, made anew when the
// moment moves.
type deadline struct {
	mu     sync.Mutex
	timer  *time.Timer
	passed chan struct{}
}

func newDeadline() *deadline {
	return &deadline{passed: make(chan struct{})}
}

// set moves the deadline to t. A zero t is no deadline.
func (d *deadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		// the timer fired, or is firing: wait for it to have closed the
		// channel, which is about to be replaced.
		<-d.passed
	}

	d.timer = nil

	closed := isClosed(d.passed)

	if t.IsZero() {
		if closed {
			d.passed = make(chan struct{})
		}

		return
	}

	if until := time.Until(t); until > 0 {
		if closed {
			d.passed = make(chan struct{})
		}

		passed := d.passed
		d.timer = time.AfterFunc(until, func() {
			close(passed)
		})

		return
	}

	if !closed {
		close(d.passed)
	}
}

// stop lets go of the timer, for a connection that is closed.
func (d *deadline) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

// wait is what is closed once the deadline has passed.
func (d *deadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.passed
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
