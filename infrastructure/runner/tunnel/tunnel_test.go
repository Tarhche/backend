package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 1. Worker registration
func TestRegistration(t *testing.T) {
	t.Run("a worker that connects becomes reachable, and says how much of it there is", func(t *testing.T) {
		config := testConfig()
		ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
		startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

		waitFor(t, "the worker to bring its pool up", func() bool {
			workers := ingress.Workers()

			return len(workers) == 1 && workers[0].Sessions == config.MinSessions
		})

		state := ingress.Workers()[0]
		assert.Equal(t, "worker-a", state.Worker)
		assert.Equal(t, config.MinSessions, state.Sessions)
		assert.Equal(t, config.MinSessions*config.MaxStreamsPerSession, state.Capacity)
		assert.Zero(t, state.Streams)
	})

	t.Run("a worker that never connected is not there", func(t *testing.T) {
		ingress := startIngress(t, testConfig(), NewTokenAuthenticator(testToken))

		_, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		assert.ErrorIs(t, err, ErrNoSuchWorker)
	})
}

// 2 & 20. Worker authentication, and failures
func TestAuthentication(t *testing.T) {
	t.Run("a worker without the token never registers", func(t *testing.T) {
		config := testConfig()
		ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

		startWorker(t, "worker-a", []string{ingress.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
			WithToken("the wrong secret"),
		)

		// it keeps trying and keeps being turned away
		waitFor(t, "the ingress to refuse it", func() bool {
			return ingress.metrics.AuthFailures.Load() > 0
		})

		assert.Empty(t, ingress.Workers())
	})

	t.Run("an empty token is refused rather than treated as none required", func(t *testing.T) {
		config := testConfig()
		ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

		startWorker(t, "worker-a", []string{ingress.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
			WithToken(""),
		)

		waitFor(t, "the ingress to refuse it", func() bool {
			return ingress.metrics.AuthFailures.Load() > 0
		})

		assert.Empty(t, ingress.Workers())
	})

	t.Run("what the ingress may do with a worker is settled when it is let in", func(t *testing.T) {
		config := testConfig()

		auth := NewTokenAuthenticator(testToken)
		auth.MaxSessions = 1

		ingress := startIngress(t, config, auth)
		startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

		waitFor(t, "the worker to get its one session in", func() bool {
			workers := ingress.Workers()

			return len(workers) == 1
		})

		// the worker wants two; it is allowed one
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 1, ingress.Workers()[0].Sessions)
	})
}

// 3. Multiple workers, and 6/7. bidirectional traffic reaching the right target
func TestMultipleWorkers(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	// each worker offers the same service name, pointing at a target of its own
	for _, name := range []string{"worker-a", "worker-b", "worker-c"} {
		startWorker(t, name, []string{ingress.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}),
		)
	}

	waitFor(t, "all three workers", func() bool { return len(ingress.Workers()) == 3 })

	for _, name := range []string{"worker-a", "worker-b", "worker-c"} {
		conn, err := ingress.Dial(t.Context(), name, Target{Service: "echo"})
		require.NoError(t, err)

		greeting := make([]byte, len(name)+1)
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(conn, greeting)
		require.NoError(t, err)

		assert.Equal(t, name+":", string(greeting), "the stream reached the wrong worker's target")
		assert.Equal(t, "hello", roundTrip(t, conn, "hello"))

		conn.Close()
	}
}

// 4 & 5. Multiple sessions per worker, multiple streams per session
func TestSessionsAndStreams(t *testing.T) {
	config := testConfig()
	config.MinSessions = 3
	config.MaxStreamsPerSession = 4

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "three sessions", func() bool {
		workers := ingress.Workers()

		return len(workers) == 1 && workers[0].Sessions == 3
	})

	// fill every session and check the streams were spread rather than piled on
	var conns []net.Conn
	for range 9 {
		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		require.NoError(t, err)

		conns = append(conns, conn)
	}

	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	sessions, err := ingress.Registry().Sessions("worker-a")
	require.NoError(t, err)

	for _, session := range sessions {
		assert.LessOrEqual(t, session.Streams(), config.MaxStreamsPerSession)
		assert.Positive(t, session.Streams(), "every session should have taken a share")
	}

	// and every one of them works
	for i, conn := range conns {
		message := fmt.Sprintf("stream-%d", i)
		assert.Equal(t, message, roundTrip(t, conn, message))
	}
}

// 6. ssh-like long-lived connections, exchanging in both directions over time
func TestLongLivedConnection(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	// a target that talks first and then answers each line, the way a shell does
	address := targetServer(t, func(conn net.Conn) {
		defer conn.Close()

		if _, err := io.WriteString(conn, "SSH-2.0-test\r\n"); err != nil {
			return
		}

		buffer := make([]byte, 64)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				return
			}

			if _, err := conn.Write(append([]byte("> "), buffer[:n]...)); err != nil {
				return
			}
		}
	})

	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"ssh": address}))
	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "ssh"})
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))

	banner := make([]byte, len("SSH-2.0-test\r\n"))
	_, err = io.ReadFull(conn, banner)
	require.NoError(t, err)
	assert.Equal(t, "SSH-2.0-test\r\n", string(banner))

	// the connection stays up across many exchanges with pauses between them,
	// which is where a keepalive that was wrong would show up
	for i := range 5 {
		message := fmt.Sprintf("command %d\n", i)

		_, err := io.WriteString(conn, message)
		require.NoError(t, err)

		answer := make([]byte, len(message)+2)
		_, err = io.ReadFull(conn, answer)
		require.NoError(t, err)

		assert.Equal(t, "> "+message, string(answer))

		time.Sleep(300 * time.Millisecond) // longer than the keepalive interval
	}
}

// 8. High concurrency
func TestHighConcurrency(t *testing.T) {
	config := testConfig()
	config.MinSessions = 4
	config.MaxSessions = 8
	config.MaxStreamsPerSession = 32

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	const clients = 100

	var (
		wait      sync.WaitGroup
		succeeded atomic.Int64
	)

	for i := range clients {
		wait.Add(1)

		go func() {
			defer wait.Done()

			conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
			if err != nil {
				return
			}
			defer conn.Close()

			message := fmt.Sprintf("client-%03d", i)

			if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}

			if _, err := io.WriteString(conn, message); err != nil {
				return
			}

			answer := make([]byte, len(message))
			if _, err := io.ReadFull(conn, answer); err != nil {
				return
			}

			if string(answer) == message {
				succeeded.Add(1)
			}
		}()
	}

	wait.Wait()

	assert.EqualValues(t, clients, succeeded.Load(), "every client should have reached the target and been answered")
}

// 9 & 14. Worker disconnect, and becoming available again
func TestWorkerDisconnect(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	address := echoServer(t, "")
	worker := startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": address}))

	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	worker.stop()

	// nothing announced this: the ingress found out because the connections it
	// was holding went.
	waitFor(t, "the worker to stop being reachable", func() bool { return len(ingress.Workers()) == 0 })

	_, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	assert.ErrorIs(t, err, ErrNoSuchWorker)

	// and a new one under the same name is reachable again
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": address}))
	waitFor(t, "the worker to come back", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "back", roundTrip(t, conn, "back"))
}

// 10. One session dying takes only its own streams with it
func TestSessionIsolation(t *testing.T) {
	config := testConfig()
	config.MinSessions = 2
	config.MaxSessions = 2
	config.MaxStreamsPerSession = 4

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "two sessions", func() bool {
		workers := ingress.Workers()

		return len(workers) == 1 && workers[0].Sessions == 2
	})

	// one connection on each session
	sessions, err := ingress.Registry().Sessions("worker-a")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	conns := make(map[string]net.Conn, 2)
	for range 2 {
		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		require.NoError(t, err)

		for _, session := range sessions {
			if session.Streams() == 1 && conns[session.ID()] == nil {
				conns[session.ID()] = conn

				break
			}
		}
	}
	require.Len(t, conns, 2, "the two streams should have landed on different sessions")

	// kill one session
	doomed := sessions[0]
	survivor := sessions[1]
	require.NoError(t, doomed.Close())

	// the stream on it fails, the way a TCP connection fails
	dead := conns[doomed.ID()]
	require.NoError(t, dead.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = io.WriteString(dead, "gone")
	if err == nil {
		_, err = io.ReadFull(dead, make([]byte, 4))
	}
	assert.Error(t, err, "the stream on the dead session should have failed")

	// the one beside it does not
	alive := conns[survivor.ID()]
	assert.Equal(t, "still here", roundTrip(t, alive, "still here"))

	// and new streams go to what is left
	waitFor(t, "the dead session to be taken out", func() bool {
		workers := ingress.Workers()

		return len(workers) == 1 && workers[0].Sessions >= 1
	})

	fresh, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer fresh.Close()

	assert.Equal(t, "new", roundTrip(t, fresh, "new"))
}

// 11 & 12. Ingress restart, and the worker reconnecting to it
func TestIngressRestart(t *testing.T) {
	config := testConfig()

	// the ingress is restarted on the same address, which is what a deploy does
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()

	first, err := NewIngress(config, NewTokenAuthenticator(testToken), discardLogger())
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)

		_ = first.Serve(t.Context(), listener)
	}()

	startWorker(t, "worker-a", []string{address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the worker", func() bool { return len(first.Workers()) == 1 })

	// the ingress goes
	listener.Close()
	first.Close()
	<-firstDone

	// and comes back on the same address
	again, err := net.Listen("tcp", address)
	require.NoError(t, err)

	second := startIngressOn(t, again, config, NewTokenAuthenticator(testToken))

	// the worker finds its way back on its own, without being told
	waitFor(t, "the worker to reconnect", func() bool {
		workers := second.Workers()

		return len(workers) == 1 && workers[0].Sessions == config.MinSessions
	})

	conn, err := second.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "after the restart", roundTrip(t, conn, "after the restart"))
}

// 13. Every session at capacity
func TestAtCapacity(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1
	config.MaxSessions = 1
	config.MaxStreamsPerSession = 2
	config.CapacityWait = 200 * time.Millisecond

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	var held []net.Conn
	for range 2 {
		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		require.NoError(t, err)

		held = append(held, conn)
	}

	// the worker cannot grow past one session, so there is nowhere to put this
	_, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	assert.ErrorIs(t, err, ErrAtCapacity)

	// when a place is given back, the next one gets it
	held[0].Close()

	waitFor(t, "the place to be given back", func() bool {
		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		if err != nil {
			return false
		}

		conn.Close()

		return true
	})
}

// The worker grows its pool before it is full, since the ingress cannot make room
func TestPoolGrows(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1
	config.MaxSessions = 4
	config.MaxStreamsPerSession = 4
	config.GrowThreshold = 0.5

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))
	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the first session", func() bool { return len(ingress.Workers()) == 1 })

	var held []net.Conn
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()

	// take enough to cross the threshold on what is open
	for range 3 {
		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		require.NoError(t, err)

		held = append(held, conn)
	}

	waitFor(t, "the worker to open another session", func() bool {
		workers := ingress.Workers()

		return len(workers) == 1 && workers[0].Sessions > 1
	})
}

// 15, 16 & 17. Backpressure, against a slow target and a slow client
func TestBackpressure(t *testing.T) {
	t.Run("a target that does not read stops the tunnel taking more", func(t *testing.T) {
		config := testConfig()
		config.MaxStreamBuffer = 64 * 1024
		config.MaxReceiveBuffer = 256 * 1024

		ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

		// a target that accepts and then reads nothing at all
		blocked := make(chan struct{})
		address := targetServer(t, func(conn net.Conn) {
			defer conn.Close()

			<-blocked
		})

		startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"slow": address}))
		waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "slow"})
		require.NoError(t, err)
		defer conn.Close()
		defer close(blocked)

		// writing far more than every buffer on the path could hold must not be
		// swallowed: it has to block once the windows are full.
		require.NoError(t, conn.SetWriteDeadline(time.Now().Add(2*time.Second)))

		written := 0
		chunk := make([]byte, 32*1024)
		for range 512 { // 16 MB, against buffers totalling well under 1 MB
			n, err := conn.Write(chunk)
			written += n

			if err != nil {
				// the write blocked until the deadline, which is the point:
				// smux refused it rather than buffering it.
				var timeout net.Error
				assert.True(t, errors.As(err, &timeout) && timeout.Timeout(),
					"the write should have blocked, not failed: %v", err)

				break
			}
		}

		assert.Less(t, written, 8*1024*1024, "the tunnel buffered far more than its windows allow")
	})

	t.Run("a client that does not read stops the target being drained", func(t *testing.T) {
		config := testConfig()
		config.MaxStreamBuffer = 64 * 1024
		config.MaxReceiveBuffer = 256 * 1024

		ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

		// a target that writes as fast as it is allowed to
		var produced atomic.Int64
		address := targetServer(t, func(conn net.Conn) {
			defer conn.Close()

			chunk := make([]byte, 32*1024)
			for {
				n, err := conn.Write(chunk)
				produced.Add(int64(n))

				if err != nil {
					return
				}
			}
		})

		startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"fast": address}))
		waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "fast"})
		require.NoError(t, err)

		// read nothing for a while, then see how much the path let through
		time.Sleep(500 * time.Millisecond)

		assert.Less(t, produced.Load(), int64(4*1024*1024),
			"a client that is not reading should have stopped the target well before this")

		conn.Close()
	})
}

// 18. Half-closed connections, which smux carries as a FIN of its own
func TestHalfClose(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	// a target that reads to EOF and only then answers, which is exactly what
	// needs the half-close to have travelled
	address := targetServer(t, func(conn net.Conn) {
		defer conn.Close()

		asked, err := io.ReadAll(conn)
		if err != nil {
			return
		}

		_, _ = fmt.Fprintf(conn, "read %d bytes", len(asked))
	})

	startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"drain": address}))
	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "drain"})
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))

	_, err = io.WriteString(conn, "question")
	require.NoError(t, err)

	// say that nothing more is coming, while staying open for the answer
	closer, ok := conn.(halfCloser)
	require.True(t, ok, "a tunnelled connection should be able to close one direction")
	require.NoError(t, closer.CloseWrite())

	answer, err := io.ReadAll(conn)
	require.NoError(t, err)

	assert.Equal(t, "read 8 bytes", string(answer), "the target never saw the end of the request")
}

// 19. Graceful shutdown: both ends stop without leaving anything running
func TestGracefulShutdown(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	worker := startWorker(t, "worker-a", []string{ingress.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))
	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	require.NoError(t, err)
	assert.Equal(t, "before", roundTrip(t, conn, "before"))

	worker.stop() // returns only when everything it started has stopped

	waitFor(t, "the ingress to let the worker go", func() bool { return len(ingress.Workers()) == 0 })

	// the stream that was open ends, rather than hanging
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = io.WriteString(conn, "after")
	if err == nil {
		_, err = io.ReadFull(conn, make([]byte, 5))
	}
	assert.Error(t, err)

	conn.Close()
	assert.NoError(t, ingress.Close())
}

// A worker connected to several ingresses is reachable through all of them,
// which is what lets there be more than one.
func TestManyIngresses(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1

	first := startIngress(t, config, NewTokenAuthenticator(testToken))
	second := startIngress(t, config, NewTokenAuthenticator(testToken))

	startWorker(t, "worker-a", []string{first.address, second.address}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
	)

	for _, ingress := range []*testIngress{first, second} {
		waitFor(t, "the worker to reach both ingresses", func() bool { return len(ingress.Workers()) == 1 })

		conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
		require.NoError(t, err)

		assert.Equal(t, "both", roundTrip(t, conn, "both"))
		conn.Close()
	}
}

// An ingress that is down does not stop the ones that are up.
func TestOneIngressDown(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	startWorker(t, "worker-a", []string{ingress.address, "127.0.0.1:1"}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
	)

	waitFor(t, "the ingress that is up", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "up", roundTrip(t, conn, "up"))
}

// The routing policy, end to end.
func TestRouting(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1
	config.MaxSessions = 1
	config.MaxStreamsPerSession = 4

	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	for _, name := range []string{"worker-a", "worker-b"} {
		startWorker(t, name, []string{ingress.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}),
		)
	}

	waitFor(t, "both workers", func() bool { return len(ingress.Workers()) == 2 })

	// the least loaded is picked, so two routed connections land one each
	reached := make(map[string]int)
	var conns []net.Conn

	for range 2 {
		conn, err := ingress.Route(t.Context(), Target{Service: "echo"})
		require.NoError(t, err)

		conns = append(conns, conn)

		greeting := make([]byte, len("worker-a:"))
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(conn, greeting)
		require.NoError(t, err)

		reached[string(greeting)]++
	}

	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	assert.Len(t, reached, 2, "the second connection should have gone to the emptier worker")
}

// A stream whose target cannot be reached says so, rather than looking like one
// that connected and closed.
func TestTargetFailures(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	startWorker(t, "worker-a", []string{ingress.address}, config,
		NewServiceTargets(map[string]string{"nothing": "127.0.0.1:1"}),
	)

	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	t.Run("a service the worker does not offer", func(t *testing.T) {
		_, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "missing"})

		assert.ErrorIs(t, err, ErrRejected)
		assert.Contains(t, err.Error(), ErrUnknownService.Error())
	})

	t.Run("an address the worker will not dial", func(t *testing.T) {
		_, err := ingress.Dial(t.Context(), "worker-a", Target{Host: "10.0.0.1", Port: 22})

		assert.ErrorIs(t, err, ErrRejected)
		assert.Contains(t, err.Error(), ErrTargetNotAllowed.Error())
	})

	t.Run("a target that is not listening", func(t *testing.T) {
		_, err := ingress.Dial(t.Context(), "worker-a", Target{Service: "nothing"})

		assert.ErrorIs(t, err, ErrRejected)
	})

	t.Run("a stream that names nothing at all", func(t *testing.T) {
		_, err := ingress.Dial(t.Context(), "worker-a", Target{})

		assert.ErrorIs(t, err, ErrProtocol)
	})

	t.Run("a place taken for a stream that failed is given back", func(t *testing.T) {
		sessions, err := ingress.Registry().Sessions("worker-a")
		require.NoError(t, err)

		for _, session := range sessions {
			assert.Zero(t, session.Streams(), "a failed stream should not hold a place")
		}
	})
}

// Explicit addresses are allowed when the worker says they are.
func TestAllowedAddresses(t *testing.T) {
	config := testConfig()
	ingress := startIngress(t, config, NewTokenAuthenticator(testToken))

	address := echoServer(t, "")
	host, port, err := net.SplitHostPort(address)
	require.NoError(t, err)

	var portNumber uint16
	_, err = fmt.Sscan(port, &portNumber)
	require.NoError(t, err)

	startWorker(t, "worker-a", []string{ingress.address}, config,
		NewServiceTargets(nil, AddressRule{Host: host, Ports: []uint16{portNumber}}),
	)

	waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

	conn, err := ingress.Dial(t.Context(), "worker-a", Target{Host: host, Port: portNumber})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "by address", roundTrip(t, conn, "by address"))

	_, err = ingress.Dial(t.Context(), "worker-a", Target{Host: host, Port: portNumber + 1})
	assert.ErrorIs(t, err, ErrRejected)
}

// startIngressOn serves an ingress on a listener the test already made, which
// is what the restart test needs.
func startIngressOn(t *testing.T, listener net.Listener, config Config, auth Authenticator) *testIngress {
	t.Helper()

	metrics := &Counters{}

	ingress, err := NewIngress(config, auth, discardLogger(), WithIngressMetrics(metrics))
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

// Every goroutine here belongs to something with an end: a session, a stream,
// a pool, an accept loop. When those end, so do they.
func TestNoGoroutineLeaks(t *testing.T) {
	settle := func() int {
		for range 100 {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		}

		return runtime.NumGoroutine()
	}

	before := settle()

	func() {
		config := testConfig()
		config.MinSessions = 2
		config.MaxStreamsPerSession = 8

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		ingress, err := NewIngress(config, NewTokenAuthenticator(testToken), discardLogger())
		require.NoError(t, err)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		served := make(chan struct{})
		go func() {
			defer close(served)

			_ = ingress.Serve(ctx, listener)
		}()

		target := echoServer(t, "")

		worker, err := NewWorker("worker-a", []string{listener.Addr().String()}, config, plainDialer(),
			NewServiceTargets(map[string]string{"echo": target}), discardLogger(), WithToken(testToken))
		require.NoError(t, err)

		stopped := make(chan struct{})
		go func() {
			defer close(stopped)

			worker.Run(ctx)
		}()

		waitFor(t, "the worker", func() bool { return len(ingress.Workers()) == 1 })

		// run a good number of streams through, then let everything go
		for range 20 {
			conn, err := ingress.Dial(ctx, "worker-a", Target{Service: "echo"})
			require.NoError(t, err)

			assert.Equal(t, "leak check", roundTrip(t, conn, "leak check"))
			conn.Close()
		}

		cancel()
		worker.Close()
		listener.Close()
		ingress.Close()

		<-stopped
		<-served
	}()

	after := settle()

	// a few may belong to the test's own echo server, which lives until the
	// test does; anything beyond a handful is something that was not cleaned up.
	assert.LessOrEqual(t, after-before, 10, "goroutines were left running: %d before, %d after", before, after)
}
