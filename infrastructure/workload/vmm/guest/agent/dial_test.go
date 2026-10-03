//go:build linux

package agent

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// echo serves a port on loopback, the task's own address in the test, that
// sends back what it is sent and says when it has nothing more to send.
func echo(t *testing.T) uint16 {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()

				_, _ = io.Copy(conn, conn)
				_ = conn.(*net.TCPConn).CloseWrite()
			}()
		}
	}()

	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

func TestDial(t *testing.T) {
	t.Run("a task's port is reached on its own address, and the connection carries bytes both ways", func(t *testing.T) {
		a := configured(t, newProcessGroups())
		a.start(t, guest.Process{Args: []string{"sleep", "60"}})

		port := echo(t)

		conn, reader, response := a.upgrade(t, "/dial?port="+strconv.Itoa(int(port)), guest.UpgradeDial, nil)
		require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

		assert.Equal(t, guest.UpgradeDial, response.Header.Get("Upgrade"))
		assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))

		_, err := conn.Write([]byte("ping"))
		require.NoError(t, err)

		// finishing sending is passed on, and the answer still comes back.
		require.NoError(t, conn.(*net.TCPConn).CloseWrite())

		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

		answer, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "ping", string(answer))
	})

	t.Run("a port nothing serves is refused, and so is anything that is not a port", func(t *testing.T) {
		a := configured(t, newProcessGroups())
		a.start(t, guest.Process{Args: []string{"sleep", "60"}})

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closed := listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		_, _, response := a.upgrade(t, "/dial?port="+strconv.Itoa(closed), guest.UpgradeDial, nil)
		assert.Equal(t, http.StatusBadGateway, response.StatusCode)

		for _, port := range []string{"0", "65536", "http", ""} {
			_, _, response := a.upgrade(t, "/dial?port="+port, guest.UpgradeDial, nil)
			assert.Equal(t, http.StatusBadRequest, response.StatusCode, port)
		}
	})

	t.Run("a task that is not running has no ports to reach", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		_, _, response := a.upgrade(t, "/dial?port="+strconv.Itoa(int(echo(t))), guest.UpgradeDial, nil)
		assert.Equal(t, http.StatusConflict, response.StatusCode)
	})

	t.Run("a task on no network has no ports anything reaches", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		config.Interfaces = nil
		require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/config", config, nil).StatusCode)

		a.start(t, guest.Process{Args: []string{"sleep", "60"}})

		_, _, response := a.upgrade(t, "/dial?port="+strconv.Itoa(int(echo(t))), guest.UpgradeDial, nil)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

// deafToHalfCloses is a connection whose half-close goes nowhere, as one over
// firecracker's vsock does: the guest saying it is done sending never reaches
// the host.
type deafToHalfCloses struct {
	net.Conn

	halfClosed atomic.Bool
}

func (c *deafToHalfCloses) CloseWrite() error {
	c.halfClosed.Store(true)

	return nil
}

func TestPipe(t *testing.T) {
	t.Run("the task ending its side ends the connection, which is all firecracker's vsock tells the host", func(t *testing.T) {
		host, agentSide := net.Pipe()
		task, upstream := net.Pipe()

		conn := &deafToHalfCloses{Conn: agentSide}

		piped := make(chan struct{})
		go func() {
			pipe(conn, conn, upstream)
			close(piped)
		}()

		// the task answers, and closes, as an HTTP/1.0 server does; the host
		// has said nothing about being done.
		go func() {
			_, _ = task.Write([]byte("bye"))
			_ = task.Close()
		}()

		require.NoError(t, host.SetReadDeadline(time.Now().Add(5*time.Second)))

		answer, err := io.ReadAll(host)
		require.NoError(t, err, "the end of the answer reached the host")
		assert.Equal(t, "bye", string(answer))

		select {
		case <-piped:
		case <-time.After(5 * time.Second):
			t.Fatal("the connection outlived both its ends")
		}
	})
}
