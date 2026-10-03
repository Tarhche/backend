//go:build linux

package agent

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The agent takes connections on a vsock port, made here on the kernel's
// sockets directly rather than through a library: the agent is in every
// machine's memory, so it carries what it needs and nothing else, and a vsock
// listener is a socket, a bind and an accept.
//
// The sockets are non-blocking and handed to the Go runtime's poller as
// files, so a connection reads, writes and keeps deadlines like any other,
// and closing the listener ends an accept that is waiting.

// fromHost admits the host and nothing else. vmhost reaches the agent through
// the socket firecracker exposes for the machine's vsock, and to the guest
// that is a connection from the host; a process inside the machine could only
// reach the agent over the kernel's own vsock loopback, as a connection from
// the machine itself, and is refused, so that what the task runs never gives
// the agent orders.
func fromHost(cid uint32) bool {
	return cid == unix.VMADDR_CID_HOST
}

// vsockAddr is one end of a vsock connection.
type vsockAddr struct {
	cid  uint32
	port uint32
}

func (a vsockAddr) Network() string {
	return "vsock"
}

func (a vsockAddr) String() string {
	return strconv.FormatUint(uint64(a.cid), 10) + ":" + strconv.FormatUint(uint64(a.port), 10)
}

// vsockListener takes the connections made to one vsock port of the machine
// from the peers it admits.
type vsockListener struct {
	file  *os.File
	raw   syscall.RawConn
	addr  vsockAddr
	admit func(cid uint32) bool
}

var _ net.Listener = (*vsockListener)(nil)

// listenVsock listens on port, for whoever admit lets in.
func listenVsock(port uint32, admit func(cid uint32) bool) (*vsockListener, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}

	if err := unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: port}); err != nil {
		unix.Close(fd)

		return nil, os.NewSyscallError("bind", err)
	}

	if err := unix.Listen(fd, unix.SOMAXCONN); err != nil {
		unix.Close(fd)

		return nil, os.NewSyscallError("listen", err)
	}

	file := os.NewFile(uintptr(fd), "vsock:"+strconv.FormatUint(uint64(port), 10))

	raw, err := file.SyscallConn()
	if err != nil {
		file.Close()

		return nil, err
	}

	return &vsockListener{file: file, raw: raw, addr: vsockAddr{cid: unix.VMADDR_CID_ANY, port: port}, admit: admit}, nil
}

// Accept waits for the next connection from a peer it admits. One it does not
// admit is closed as soon as it is taken.
func (l *vsockListener) Accept() (net.Conn, error) {
	for {
		var (
			fd        int
			peer      unix.Sockaddr
			acceptErr error
		)

		err := l.raw.Read(func(socket uintptr) bool {
			fd, peer, acceptErr = unix.Accept4(int(socket), unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)

			// a socket with nothing to accept is waited on, rather than
			// asked again at once.
			return !errors.Is(acceptErr, unix.EAGAIN)
		})
		if err != nil {
			return nil, &net.OpError{Op: "accept", Net: "vsock", Addr: l.addr, Err: err}
		}

		if acceptErr != nil {
			if errors.Is(acceptErr, unix.ECONNABORTED) || errors.Is(acceptErr, unix.EINTR) {
				continue
			}

			// what the runtime takes for a passing trouble, too many open
			// files among them, an http server waits out rather than
			// ending on.
			return nil, &net.OpError{Op: "accept", Net: "vsock", Addr: l.addr, Err: os.NewSyscallError("accept4", acceptErr)}
		}

		remote, ok := peer.(*unix.SockaddrVM)
		if !ok || !l.admit(remote.CID) {
			unix.Close(fd)

			continue
		}

		return &vsockConn{
			file:   os.NewFile(uintptr(fd), "vsock"),
			local:  l.addr,
			remote: vsockAddr{cid: remote.CID, port: remote.Port},
		}, nil
	}
}

// Close stops the listener, and ends an Accept that is waiting.
func (l *vsockListener) Close() error {
	return l.file.Close()
}

func (l *vsockListener) Addr() net.Addr {
	return l.addr
}

// vsockConn is one connection on a vsock port.
type vsockConn struct {
	file   *os.File
	local  vsockAddr
	remote vsockAddr
}

var _ net.Conn = (*vsockConn)(nil)

func (c *vsockConn) Read(p []byte) (int, error) {
	n, err := c.file.Read(p)

	return n, c.wrap("read", err)
}

func (c *vsockConn) Write(p []byte) (int, error) {
	n, err := c.file.Write(p)

	return n, c.wrap("write", err)
}

// wrap says what went wrong the way a connection of the net package does.
// An http server tells a read it interrupted itself, once an answer is done,
// from a peer that went away by the error being a net.Error that timed out. A
// file's error is neither, so the server would take every connection for gone
// after its first answer, and cancel whatever was asked on it next.
func (c *vsockConn) wrap(op string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		return io.EOF
	}

	var path *os.PathError
	if errors.As(err, &path) {
		err = path.Err
	}

	return &net.OpError{Op: op, Net: "vsock", Source: c.local, Addr: c.remote, Err: err}
}

func (c *vsockConn) Close() error {
	return c.file.Close()
}

// CloseWrite says nothing more will be sent, while what the peer sends can
// still be read.
func (c *vsockConn) CloseWrite() error {
	raw, err := c.file.SyscallConn()
	if err != nil {
		return err
	}

	var shutdownErr error
	if err := raw.Control(func(socket uintptr) {
		shutdownErr = unix.Shutdown(int(socket), unix.SHUT_WR)
	}); err != nil {
		return err
	}

	return os.NewSyscallError("shutdown", shutdownErr)
}

func (c *vsockConn) LocalAddr() net.Addr {
	return c.local
}

func (c *vsockConn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *vsockConn) SetDeadline(t time.Time) error {
	return c.file.SetDeadline(t)
}

func (c *vsockConn) SetReadDeadline(t time.Time) error {
	return c.file.SetReadDeadline(t)
}

func (c *vsockConn) SetWriteDeadline(t time.Time) error {
	return c.file.SetWriteDeadline(t)
}
