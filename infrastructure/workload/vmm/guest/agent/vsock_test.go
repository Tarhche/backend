//go:build linux

package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// The listener is tried over the kernel's vsock loopback, where a machine's
// own processes would reach it from: a kernel without one (vsock_loopback)
// skips these.

// vsockPort is a port nothing else in the test listens on.
const vsockPort = 0x5b2c

// dialLocal connects to port over the vsock loopback.
func dialLocal(t *testing.T, port uint32) net.Conn {
	t.Helper()

	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	require.NoError(t, err)

	if err := unix.Connect(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_LOCAL, Port: port}); err != nil {
		unix.Close(fd)
		t.Fatalf("connecting over the vsock loopback: %v", err)
	}

	// the net package takes no vsock sockets, so the client end is one the
	// way the listener's are: non-blocking, in the runtime's poller.
	require.NoError(t, unix.SetNonblock(fd, true))

	return &vsockConn{
		file:   os.NewFile(uintptr(fd), "vsock"),
		remote: vsockAddr{cid: unix.VMADDR_CID_LOCAL, port: port},
	}
}

// listenLocal listens on the vsock loopback for whoever admit lets in, or
// skips the test where there is no loopback to listen on.
func listenLocal(t *testing.T, admit func(cid uint32) bool) *vsockListener {
	t.Helper()

	// a kernel without vsock, or a sandbox that refuses it (docker's seccomp
	// profile does), makes no vsock socket at all.
	socket, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("no vsock socket can be made here: %v", err)
	}

	unix.Close(socket)

	listener, err := listenVsock(vsockPort, admit)
	if errors.Is(err, unix.EADDRNOTAVAIL) || errors.Is(err, unix.ENODEV) {
		t.Skipf("the kernel has no vsock to listen on: %v", err)
	}

	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	// a connection over the loopback needs its transport, which a kernel
	// may not have loaded.
	probe, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	defer unix.Close(probe)

	if err := unix.Connect(probe, &unix.SockaddrVM{CID: unix.VMADDR_CID_LOCAL, Port: vsockPort}); err != nil {
		t.Skipf("the kernel has no vsock loopback (modprobe vsock_loopback): %v", err)
	}

	// the probe is taken, if it is admitted, and let go of.
	if admit(unix.VMADDR_CID_LOCAL) {
		conn, err := listener.Accept()
		require.NoError(t, err)
		conn.Close()
	}

	return listener
}

func TestVsock(t *testing.T) {
	t.Run("a connection from an admitted peer carries bytes both ways, and ends each way on its own", func(t *testing.T) {
		listener := listenLocal(t, func(cid uint32) bool { return cid == unix.VMADDR_CID_LOCAL })

		accepted := make(chan net.Conn, 1)
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				accepted <- conn
			}
		}()

		client := dialLocal(t, vsockPort)
		defer client.Close()

		var server net.Conn
		select {
		case server = <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatal("the connection was not accepted")
		}
		defer server.Close()

		assert.Equal(t, "vsock", server.RemoteAddr().Network())
		assert.Equal(t, vsockAddr{cid: unix.VMADDR_CID_ANY, port: vsockPort}, server.LocalAddr())

		_, err := client.Write([]byte("ping"))
		require.NoError(t, err)

		buffer := make([]byte, 4)
		require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(server, buffer)
		require.NoError(t, err)
		assert.Equal(t, "ping", string(buffer))

		require.NoError(t, server.(*vsockConn).CloseWrite())

		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
		rest, err := io.ReadAll(client)
		require.NoError(t, err)
		assert.Empty(t, rest, "the server finished sending")

		_, err = client.Write([]byte("still"))
		require.NoError(t, err, "while the client may still send")

		_, err = io.ReadFull(server, buffer[:4])
		require.NoError(t, err)
	})

	t.Run("a peer that is not the host is refused", func(t *testing.T) {
		listener := listenLocal(t, fromHost)

		accepted := make(chan net.Conn, 1)
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				accepted <- conn
			}
		}()

		client := dialLocal(t, vsockPort)
		defer client.Close()

		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))

		_, err := client.Read(make([]byte, 1))
		assert.Error(t, err, "the connection was closed as soon as it was taken")

		select {
		case <-accepted:
			t.Fatal("a process inside the machine reached the agent")
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("a read interrupted by its deadline says it timed out, as a network connection's does", func(t *testing.T) {
		listener := listenLocal(t, func(cid uint32) bool { return cid == unix.VMADDR_CID_LOCAL })

		accepted := make(chan net.Conn, 1)
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				accepted <- conn
			}
		}()

		client := dialLocal(t, vsockPort)

		var server net.Conn
		select {
		case server = <-accepted:
		case <-time.After(5 * time.Second):
			t.Fatal("the connection was not accepted")
		}
		defer server.Close()

		require.NoError(t, server.SetReadDeadline(time.Now().Add(50*time.Millisecond)))

		_, err := server.Read(make([]byte, 1))

		// an http server asks the error itself, not what it wraps, whether
		// it is a net.Error that timed out: that is how it tells a read it
		// interrupted from a peer that left.
		timedOut, ok := err.(net.Error) //nolint:errorlint // what net/http does
		require.True(t, ok, "%T is no net.Error", err)
		assert.True(t, timedOut.Timeout())
		assert.ErrorIs(t, err, os.ErrDeadlineExceeded)

		require.NoError(t, client.Close())
		require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))

		_, err = server.Read(make([]byte, 1))
		assert.Equal(t, io.EOF, err, "a peer that is done is io.EOF itself")
	})

	t.Run("the agent is served on it, request after request on one connection", func(t *testing.T) {
		listener := listenLocal(t, func(cid uint32) bool { return cid == unix.VMADDR_CID_LOCAL })

		a := newTestAgent(t, newProcessGroups())

		server := &http.Server{Handler: a.routes(), ReadHeaderTimeout: readHeaderTimeout}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { server.Close() })

		var dials atomic.Int32
		client := &http.Client{Transport: &http.Transport{
			Dial: func(string, string) (net.Conn, error) {
				dials.Add(1)

				return dialLocal(t, vsockPort), nil
			},
			MaxConnsPerHost: 1,
		}}
		t.Cleanup(client.CloseIdleConnections)

		do := func(method string, path string, body any) (*http.Response, []byte) {
			t.Helper()

			var payload io.Reader
			if body != nil {
				encoded, err := json.Marshal(body)
				require.NoError(t, err)

				payload = bytes.NewReader(encoded)
			}

			request, err := http.NewRequest(method, "http://agent"+path, payload)
			require.NoError(t, err)

			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()

			content, err := io.ReadAll(response.Body)
			require.NoError(t, err)

			return response, content
		}

		response, _ := do(http.MethodGet, "/health", nil)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)
		assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))

		response, _ = do(http.MethodPut, "/config", testConfig())
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		response, _ = do(http.MethodPost, "/process", guest.Process{Args: []string{"sleep", "0.3"}})
		require.Equal(t, http.StatusOK, response.StatusCode)

		// a request that waits is answered once there is something to say, on
		// the connection the others were asked on: a server that took the
		// connection for gone after its first answer cancels it at once, and
		// says nothing.
		began := time.Now()
		response, content := do(http.MethodGet, "/process/wait?generation=1", nil)
		require.Equal(t, http.StatusOK, response.StatusCode)

		var status guest.Status
		require.NoError(t, json.Unmarshal(content, &status), "the wait was answered: %q", content)
		assert.Equal(t, guest.StateExited, status.State)
		assert.Equal(t, 0, status.ExitCode)
		assert.GreaterOrEqual(t, time.Since(began), 100*time.Millisecond, "it waited for the task")

		assert.Equal(t, int32(1), dials.Load(), "every request went over the one connection")
	})

	t.Run("closing the listener ends an accept that is waiting", func(t *testing.T) {
		listener := listenLocal(t, fromHost)

		done := make(chan error, 1)
		go func() {
			_, err := listener.Accept()
			done <- err
		}()

		time.Sleep(50 * time.Millisecond)
		require.NoError(t, listener.Close())

		select {
		case err := <-done:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("the accept went on waiting")
		}
	})
}
