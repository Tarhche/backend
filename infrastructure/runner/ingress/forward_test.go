package ingress

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// fakeLister stands in for the tasks the runner is holding. It is read by the
// forwarder's own goroutine while a test changes it, so it holds a lock.
type fakeLister struct {
	mutex sync.Mutex
	tasks []task.Task
	err   error
}

func (f *fakeLister) GetRunningWithPublicPorts(context.Context) ([]task.Task, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return f.tasks, f.err
}

func (f *fakeLister) hold(tasks []task.Task) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.tasks = tasks
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// echoTCP is a container answering on tcp, and the port it came up on.
func echoTCP(t *testing.T) port.Port {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()

				reader := bufio.NewReader(conn)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}

					_, _ = fmt.Fprintf(conn, "tcp: %s", line)
				}
			}()
		}
	}()

	return portOf(t, listener.Addr().String())
}

// echoUDP is the same container answering on udp.
func echoUDP(t *testing.T) port.Port {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buffer := make([]byte, 1024)

		for {
			read, from, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}

			_, _ = conn.WriteTo(append([]byte("udp: "), buffer[:read]...), from)
		}
	}()

	return portOf(t, conn.LocalAddr().String())
}

func portOf(t *testing.T, address string) port.Port {
	t.Helper()

	_, value, err := net.SplitHostPort(address)
	require.NoError(t, err)

	number, err := strconv.Atoi(value)
	require.NoError(t, err)

	return port.Port(number)
}

// free is a port nothing is listening on, for the forwarder to take.
func free(t *testing.T) port.Port {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	taken := portOf(t, listener.Addr().String())
	require.NoError(t, listener.Close())

	return taken
}

func TestForwarder(t *testing.T) {
	t.Parallel()

	t.Run("carries a whole conversation to a container, both ways", func(t *testing.T) {
		t.Parallel()

		var (
			tcpPort = echoTCP(t)
			udpPort = echoUDP(t)
			public  = free(t)
		)

		lister := &fakeLister{}
		lister.hold([]task.Task{{
			CurrentState: task.Running,
			Endpoints: []task.Endpoint{{
				ContainerPort: 9000,
				Host:          "127.0.0.1",
				HostPort:      tcpPort,
				HostPortUDP:   udpPort,
				PublicPort:    public,
			}},
		}})

		forwarder, err := NewForwarder(lister, fmt.Sprintf("%d-%d", public, public), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go forwarder.Serve(ctx, 50*time.Millisecond)

		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(public)))
		requireListening(t, address)

		t.Run("tcp", func(t *testing.T) {
			conn, err := net.DialTimeout("tcp", address, time.Second)
			require.NoError(t, err)
			defer conn.Close()

			_, err = fmt.Fprintln(conn, "hello")
			require.NoError(t, err)

			require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

			answer, err := bufio.NewReader(conn).ReadString('\n')
			require.NoError(t, err)
			assert.Equal(t, "tcp: hello\n", answer)
		})

		t.Run("udp", func(t *testing.T) {
			conn, err := net.Dial("udp", address)
			require.NoError(t, err)
			defer conn.Close()

			_, err = conn.Write([]byte("hello"))
			require.NoError(t, err)

			require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

			buffer := make([]byte, 64)
			read, err := conn.Read(buffer)
			require.NoError(t, err)
			assert.Equal(t, "udp: hello", string(buffer[:read]))
		})
	})

	t.Run("lets go of a port when its container is gone", func(t *testing.T) {
		t.Parallel()

		var (
			tcpPort = echoTCP(t)
			public  = free(t)
		)

		lister := &fakeLister{}
		lister.hold([]task.Task{{
			CurrentState: task.Running,
			Endpoints: []task.Endpoint{{
				ContainerPort: 9000,
				Host:          "127.0.0.1",
				HostPort:      tcpPort,
				PublicPort:    public,
			}},
		}})

		forwarder, err := NewForwarder(lister, fmt.Sprintf("%d-%d", public, public), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go forwarder.Serve(ctx, 50*time.Millisecond)

		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(public)))
		requireListening(t, address)

		lister.hold(nil)

		require.Eventually(t, func() bool {
			conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
			if err != nil {
				return true
			}

			_ = conn.Close()

			return false
		}, 3*time.Second, 50*time.Millisecond, "the port is still being listened on")
	})

	t.Run("a range nobody can read is a mistake worth reporting", func(t *testing.T) {
		t.Parallel()

		_, err := NewForwarder(&fakeLister{}, "thirty thousand", discardLogger())
		assert.Error(t, err)
	})

	t.Run("forwarding nothing is a way to run", func(t *testing.T) {
		t.Parallel()

		forwarder, err := NewForwarder(&fakeLister{}, "", discardLogger())
		require.NoError(t, err)
		assert.NotNil(t, forwarder)
	})
}

func requireListening(t *testing.T, address string) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err != nil {
			return false
		}

		_ = conn.Close()

		return true
	}, 3*time.Second, 50*time.Millisecond, "the forwarder never listened")
}
