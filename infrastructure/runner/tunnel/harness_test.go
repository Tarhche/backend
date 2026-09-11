package tunnel

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testToken = "a-shared-secret"

// testConfig is the defaults made small and quick, so that a test exercises the
// same code paths without waiting for production timings.
func testConfig() Config {
	c := DefaultConfig()
	c.MinSessions = 2
	c.MaxSessions = 4
	c.MaxStreamsPerSession = 8
	c.KeepAliveInterval = 200 * time.Millisecond
	c.KeepAliveTimeout = 600 * time.Millisecond
	c.HandshakeTimeout = 2 * time.Second
	c.DialTimeout = 2 * time.Second
	c.CapacityWait = 500 * time.Millisecond
	c.ReconnectMinDelay = 10 * time.Millisecond
	c.ReconnectMaxDelay = 100 * time.Millisecond
	c.IdleSessionTimeout = 0

	return c
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// testIngress is an ingress listening on the loopback, torn down with the test.
type testIngress struct {
	*Ingress

	address  string
	listener net.Listener
	metrics  *Counters
	done     chan struct{}
}

func startIngress(t *testing.T, config Config, auth Authenticator, options ...IngressOption) *testIngress {
	t.Helper()

	metrics := &Counters{}
	options = append([]IngressOption{WithIngressMetrics(metrics)}, options...)

	ingress, err := NewIngress(config, auth, discardLogger(), options...)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = ingress.Serve(ctx, listener)
	}()

	t.Cleanup(func() {
		cancel()
		ingress.Close()
		<-done
	})

	return &testIngress{Ingress: ingress, address: listener.Addr().String(), listener: listener, metrics: metrics, done: done}
}

// plainDialer reaches an ingress over unencrypted tcp, which is what the tests
// that are not about TLS use.
func plainDialer() Dialer {
	return DialerFunc(func(ctx context.Context, address string) (net.Conn, error) {
		dialer := net.Dialer{}

		return dialer.DialContext(ctx, "tcp", address)
	})
}

type testWorker struct {
	*Worker

	metrics *Counters
	stopped chan struct{}
	cancel  context.CancelFunc
}

func startWorker(t *testing.T, id string, addresses []string, config Config, targets Targets, options ...WorkerOption) *testWorker {
	t.Helper()

	metrics := &Counters{}
	options = append([]WorkerOption{WithToken(testToken), WithWorkerMetrics(metrics)}, options...)

	worker, err := NewWorker(id, addresses, config, plainDialer(), targets, discardLogger(), options...)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		worker.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		worker.Close()
		<-stopped
	})

	return &testWorker{Worker: worker, metrics: metrics, stopped: stopped, cancel: cancel}
}

// stop ends a worker the way a shutdown does, and waits for it to be over.
func (w *testWorker) stop() {
	w.cancel()
	w.Close()
	<-w.stopped
}

// echoServer answers with what it was sent, which is enough to prove that bytes
// reached the right target and came back.
func echoServer(t *testing.T, prefix string) string {
	t.Helper()

	return targetServer(t, func(conn net.Conn) {
		defer conn.Close()

		if len(prefix) > 0 {
			if _, err := io.WriteString(conn, prefix); err != nil {
				return
			}
		}

		_, _ = io.Copy(conn, conn)
	})
}

// targetServer runs handle for every connection until the test ends.
func targetServer(t *testing.T, handle func(net.Conn)) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var running sync.WaitGroup

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			running.Add(1)

			go func() {
				defer running.Done()

				handle(conn)
			}()
		}
	}()

	t.Cleanup(func() {
		listener.Close()
		running.Wait()
	})

	return listener.Addr().String()
}

// waitFor gives the pools a moment to settle, since connecting is asynchronous.
func waitFor(t *testing.T, why string, condition func() bool) {
	t.Helper()

	for range 400 {
		if condition() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting: %s", why)
}

// roundTrip writes a line and reads the answer, which is what most of these
// tests do to prove a stream works.
func roundTrip(t *testing.T, conn net.Conn, message string) string {
	t.Helper()

	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err := io.WriteString(conn, message)
	require.NoError(t, err)

	answer := make([]byte, len(message))
	_, err = io.ReadFull(conn, answer)
	require.NoError(t, err)

	return string(answer)
}
