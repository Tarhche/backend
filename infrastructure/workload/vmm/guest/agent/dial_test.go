//go:build linux

package agent

import (
	"io"
	"net"
	"net/http"
	"strconv"
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
