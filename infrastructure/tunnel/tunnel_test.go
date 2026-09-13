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

// 1. Agent registration
func TestRegistration(t *testing.T) {
	t.Run("an agent that connects becomes reachable, and says how much of it there is", func(t *testing.T) {
		config := testConfig()
		hub := startHub(t, config, AllowAll())
		startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

		waitFor(t, "the agent to bring its pool up", func() bool {
			agents := hub.Agents()

			return len(agents) == 1 && agents[0].Sessions == config.MinSessions
		})

		state := hub.Agents()[0]
		assert.Equal(t, "agent-a", state.Name)
		assert.Equal(t, config.MinSessions, state.Sessions)
		assert.Equal(t, config.MinSessions*config.MaxStreamsPerSession, state.Capacity)
		assert.Zero(t, state.Streams)
	})

	t.Run("an agent that never connected is not there", func(t *testing.T) {
		hub := startHub(t, testConfig(), AllowAll())

		_, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		assert.ErrorIs(t, err, ErrNoSuchAgent)
	})
}

// 3. Multiple agents, and 6/7. bidirectional traffic reaching the right target
func TestMultipleAgents(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	// each agent offers the same service name, pointing at a target of its own
	for _, name := range []string{"agent-a", "agent-b", "agent-c"} {
		startAgent(t, name, []string{hub.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}),
		)
	}

	waitFor(t, "all three agents", func() bool { return len(hub.Agents()) == 3 })

	for _, name := range []string{"agent-a", "agent-b", "agent-c"} {
		conn, err := hub.Dial(t.Context(), name, Target{Service: "echo"})
		require.NoError(t, err)

		greeting := make([]byte, len(name)+1)
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(conn, greeting)
		require.NoError(t, err)

		assert.Equal(t, name+":", string(greeting), "the stream reached the wrong agent's target")
		assert.Equal(t, "hello", roundTrip(t, conn, "hello"))

		conn.Close()
	}
}

// 4 & 5. Multiple sessions per agent, multiple streams per session
func TestSessionsAndStreams(t *testing.T) {
	config := testConfig()
	config.MinSessions = 3
	config.MaxStreamsPerSession = 4

	hub := startHub(t, config, AllowAll())
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "three sessions", func() bool {
		agents := hub.Agents()

		return len(agents) == 1 && agents[0].Sessions == 3
	})

	// fill every session and check the streams were spread rather than piled on
	var conns []net.Conn
	for range 9 {
		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		require.NoError(t, err)

		conns = append(conns, conn)
	}

	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	sessions, err := hub.Registry().Sessions("agent-a")
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
	hub := startHub(t, config, AllowAll())

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

	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"ssh": address}))
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "ssh"})
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

	hub := startHub(t, config, AllowAll())
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	const clients = 100

	var (
		wait      sync.WaitGroup
		succeeded atomic.Int64
	)

	for i := range clients {

		wait.Go(func() {

			conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
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
		})
	}

	wait.Wait()

	assert.EqualValues(t, clients, succeeded.Load(), "every client should have reached the target and been answered")
}

// 9 & 14. Agent disconnect, and becoming available again
func TestAgentDisconnect(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	address := echoServer(t, "")
	agent := startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": address}))

	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	agent.stop()

	// nothing announced this: the hub found out because the connections it
	// was holding went.
	waitFor(t, "the agent to stop being reachable", func() bool { return len(hub.Agents()) == 0 })

	_, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
	assert.ErrorIs(t, err, ErrNoSuchAgent)

	// and a new one under the same name is reachable again
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": address}))
	waitFor(t, "the agent to come back", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
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

	hub := startHub(t, config, AllowAll())
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "two sessions", func() bool {
		agents := hub.Agents()

		return len(agents) == 1 && agents[0].Sessions == 2
	})

	// one connection on each session
	sessions, err := hub.Registry().Sessions("agent-a")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	conns := make(map[string]net.Conn, 2)
	for range 2 {
		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
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
		agents := hub.Agents()

		return len(agents) == 1 && agents[0].Sessions >= 1
	})

	fresh, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer fresh.Close()

	assert.Equal(t, "new", roundTrip(t, fresh, "new"))
}

// 11 & 12. Hub restart, and the agent reconnecting to it
func TestHubRestart(t *testing.T) {
	config := testConfig()

	// the hub is restarted on the same address, which is what a deploy does
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()

	first, err := NewHub(config, AllowAll(), discardLogger())
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)

		_ = first.Serve(t.Context(), listener)
	}()

	startAgent(t, "agent-a", []string{address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the agent", func() bool { return len(first.Agents()) == 1 })

	// the hub goes
	listener.Close()
	first.Close()
	<-firstDone

	// and comes back on the same address
	again, err := net.Listen("tcp", address)
	require.NoError(t, err)

	second := startHubOn(t, again, config, AllowAll())

	// the agent finds its way back on its own, without being told
	waitFor(t, "the agent to reconnect", func() bool {
		agents := second.Agents()

		return len(agents) == 1 && agents[0].Sessions == config.MinSessions
	})

	conn, err := second.Dial(t.Context(), "agent-a", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "after the restart", roundTrip(t, conn, "after the restart"))
}

// 12b. The backoff is forgotten the moment a connection is made, so an agent
// that spent an outage backing off does not go on waiting as if it still were.
func TestReconnectResetsBackoff(t *testing.T) {
	config := testConfig()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()

	first, err := NewHub(config, AllowAll(), discardLogger())
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)

		_ = first.Serve(t.Context(), listener)
	}()

	agent := startAgent(t, "agent-a", []string{address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the agent", func() bool { return len(first.Agents()) == 1 })
	require.Zero(t, attemptsOf(agent), "an agent that connected first time has nothing to back off from")

	// the hub goes, and stays gone long enough to be given up on more than
	// once — which is what puts the pool into a grown backoff at all
	listener.Close()
	first.Close()
	<-firstDone

	waitFor(t, "the agent to have failed more than once", func() bool {
		return attemptsOf(agent) > 1
	})

	backedOff := attemptsOf(agent)
	require.Greater(t, backedOff, 1)

	// and comes back on the same address
	again, err := net.Listen("tcp", address)
	require.NoError(t, err)

	second := startHubOn(t, again, config, AllowAll())

	waitFor(t, "the agent to reconnect", func() bool {
		agents := second.Agents()

		return len(agents) == 1 && agents[0].Sessions == config.MinSessions
	})

	waitFor(t, "the backoff to be forgotten", func() bool { return attemptsOf(agent) == 0 })

	assert.Zero(t, attemptsOf(agent),
		"it backed off %d times and reconnected; the next failure should wait from the start again", backedOff)

	// and the wait it would take now is one drawn from the first step, not from
	// wherever the outage had pushed it
	assert.LessOrEqual(t, agent.pools[0].backoff(), config.ReconnectMinDelay)
}

// attemptsOf is how many times in a row an agent has failed to reach its first
// hub, which is what the backoff is computed from.
func attemptsOf(w *testAgent) int {
	pool := w.pools[0]

	pool.lock.Lock()
	defer pool.lock.Unlock()

	return pool.attempt
}

// 13. Every session at capacity
func TestAtCapacity(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1
	config.MaxSessions = 1
	config.MaxStreamsPerSession = 2
	config.CapacityWait = 200 * time.Millisecond

	hub := startHub(t, config, AllowAll())
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	var held []net.Conn
	for range 2 {
		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		require.NoError(t, err)

		held = append(held, conn)
	}

	// the agent cannot grow past one session, so there is nowhere to put this
	_, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
	assert.ErrorIs(t, err, ErrAtCapacity)

	// when a place is given back, the next one gets it
	held[0].Close()

	waitFor(t, "the place to be given back", func() bool {
		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		if err != nil {
			return false
		}

		conn.Close()

		return true
	})
}

// The agent grows its pool before it is full, since the hub cannot make room
func TestPoolGrows(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1
	config.MaxSessions = 4
	config.MaxStreamsPerSession = 4
	config.GrowThreshold = 0.5

	hub := startHub(t, config, AllowAll())
	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))

	waitFor(t, "the first session", func() bool { return len(hub.Agents()) == 1 })

	var held []net.Conn
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()

	// take enough to cross the threshold on what is open
	for range 3 {
		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		require.NoError(t, err)

		held = append(held, conn)
	}

	waitFor(t, "the agent to open another session", func() bool {
		agents := hub.Agents()

		return len(agents) == 1 && agents[0].Sessions > 1
	})
}

// 15, 16 & 17. Backpressure, against a slow target and a slow client
func TestBackpressure(t *testing.T) {
	t.Run("a target that does not read stops the tunnel taking more", func(t *testing.T) {
		config := testConfig()
		config.MaxStreamBuffer = 64 * 1024
		config.MaxReceiveBuffer = 256 * 1024

		hub := startHub(t, config, AllowAll())

		// a target that accepts and then reads nothing at all
		blocked := make(chan struct{})
		address := targetServer(t, func(conn net.Conn) {
			defer conn.Close()

			<-blocked
		})

		startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"slow": address}))
		waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "slow"})
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

		hub := startHub(t, config, AllowAll())

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

		startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"fast": address}))
		waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "fast"})
		require.NoError(t, err)
		defer conn.Close()

		// what matters is that the target *stops*, not how much it managed
		// before it did: how much fits along the way is the machine's socket
		// buffers, which vary, while coming to a halt is the property.
		waitFor(t, "the target to be stopped by a client that is not reading", func() bool {
			before := produced.Load()
			time.Sleep(200 * time.Millisecond)

			return produced.Load() == before
		})

		stopped := produced.Load()

		// and it stays stopped, rather than trickling on
		time.Sleep(300 * time.Millisecond)
		assert.Equal(t, stopped, produced.Load(), "the target should still be blocked")

		// an unthrottled writer on loopback would be far past this by now
		assert.Less(t, stopped, int64(64*1024*1024))
	})
}

// 18. Half-closed connections, which smux carries as a FIN of its own
func TestHalfClose(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

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

	startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"drain": address}))
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "drain"})
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
	hub := startHub(t, config, AllowAll())

	agent := startAgent(t, "agent-a", []string{hub.address}, config, NewServiceTargets(map[string]string{"echo": echoServer(t, "")}))
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
	require.NoError(t, err)
	assert.Equal(t, "before", roundTrip(t, conn, "before"))

	agent.stop() // returns only when everything it started has stopped

	waitFor(t, "the hub to let the agent go", func() bool { return len(hub.Agents()) == 0 })

	// the stream that was open ends, rather than hanging
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = io.WriteString(conn, "after")
	if err == nil {
		_, err = io.ReadFull(conn, make([]byte, 5))
	}
	assert.Error(t, err)

	conn.Close()
	assert.NoError(t, hub.Close())
}

// An agent connected to several hubs is reachable through all of them,
// which is what lets there be more than one.
func TestManyHubes(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1

	first := startHub(t, config, AllowAll())
	second := startHub(t, config, AllowAll())

	startAgent(t, "agent-a", []string{first.address, second.address}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
	)

	for _, hub := range []*testHub{first, second} {
		waitFor(t, "the agent to reach both hubs", func() bool { return len(hub.Agents()) == 1 })

		conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
		require.NoError(t, err)

		assert.Equal(t, "both", roundTrip(t, conn, "both"))
		conn.Close()
	}
}

// A hub that is down does not stop the ones that are up.
func TestOneHubDown(t *testing.T) {
	config := testConfig()
	config.MinSessions = 1

	hub := startHub(t, config, AllowAll())

	startAgent(t, "agent-a", []string{hub.address, "127.0.0.1:1"}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
	)

	waitFor(t, "the hub that is up", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Service: "echo"})
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

	hub := startHub(t, config, AllowAll())

	for _, name := range []string{"agent-a", "agent-b"} {
		startAgent(t, name, []string{hub.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}),
		)
	}

	waitFor(t, "both agents", func() bool { return len(hub.Agents()) == 2 })

	// the least loaded is picked, so two routed connections land one each
	reached := make(map[string]int)
	var conns []net.Conn

	for range 2 {
		conn, err := hub.Route(t.Context(), Target{Service: "echo"})
		require.NoError(t, err)

		conns = append(conns, conn)

		greeting := make([]byte, len("agent-a:"))
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

	assert.Len(t, reached, 2, "the second connection should have gone to the emptier agent")
}

// A stream whose target cannot be reached says so, rather than looking like one
// that connected and closed.
func TestTargetFailures(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(map[string]string{"nothing": "127.0.0.1:1"}),
	)

	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	t.Run("a service the agent does not offer", func(t *testing.T) {
		_, err := hub.Dial(t.Context(), "agent-a", Target{Service: "missing"})

		assert.ErrorIs(t, err, ErrRejected)
		assert.Contains(t, err.Error(), ErrUnknownService.Error())
	})

	t.Run("an address the agent will not dial", func(t *testing.T) {
		_, err := hub.Dial(t.Context(), "agent-a", Target{Host: "10.0.0.1", Port: 22})

		assert.ErrorIs(t, err, ErrRejected)
		assert.Contains(t, err.Error(), ErrTargetNotAllowed.Error())
	})

	t.Run("a target that is not listening", func(t *testing.T) {
		_, err := hub.Dial(t.Context(), "agent-a", Target{Service: "nothing"})

		assert.ErrorIs(t, err, ErrRejected)
	})

	t.Run("a stream that names nothing at all", func(t *testing.T) {
		_, err := hub.Dial(t.Context(), "agent-a", Target{})

		assert.ErrorIs(t, err, ErrProtocol)
	})

	t.Run("a place taken for a stream that failed is given back", func(t *testing.T) {
		sessions, err := hub.Registry().Sessions("agent-a")
		require.NoError(t, err)

		for _, session := range sessions {
			assert.Zero(t, session.Streams(), "a failed stream should not hold a place")
		}
	})
}

// Explicit addresses are allowed when the agent says they are.
func TestAllowedAddresses(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	address := echoServer(t, "")
	host, port, err := net.SplitHostPort(address)
	require.NoError(t, err)

	var portNumber uint16
	_, err = fmt.Sscan(port, &portNumber)
	require.NoError(t, err)

	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(nil, AddressRule{Host: host, Ports: []uint16{portNumber}}),
	)

	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	conn, err := hub.Dial(t.Context(), "agent-a", Target{Host: host, Port: portNumber})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "by address", roundTrip(t, conn, "by address"))

	_, err = hub.Dial(t.Context(), "agent-a", Target{Host: host, Port: portNumber + 1})
	assert.ErrorIs(t, err, ErrRejected)
}

// startHubOn serves a hub on a listener the test already made, which
// is what the restart test needs.
func startHubOn(t *testing.T, listener net.Listener, config Config, auth Authenticator) *testHub {
	t.Helper()

	metrics := &Counters{}

	hub, err := NewHub(config, auth, discardLogger(), WithHubMetrics(metrics))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = hub.Serve(ctx, listener)
	}()

	t.Cleanup(func() {
		cancel()
		hub.Close()
		<-done
	})

	return &testHub{Hub: hub, address: listener.Addr().String(), listener: listener, metrics: metrics, done: done}
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

		hub, err := NewHub(config, AllowAll(), discardLogger())
		require.NoError(t, err)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		served := make(chan struct{})
		go func() {
			defer close(served)

			_ = hub.Serve(ctx, listener)
		}()

		target := echoServer(t, "")

		agent, err := NewAgent("agent-a", []string{listener.Addr().String()}, config, plainDialer(),
			NewServiceTargets(map[string]string{"echo": target}), discardLogger(), WithAgentMetrics(&Counters{}))
		require.NoError(t, err)

		stopped := make(chan struct{})
		go func() {
			defer close(stopped)

			agent.Run(ctx)
		}()

		waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

		// run a good number of streams through, then let everything go
		for range 20 {
			conn, err := hub.Dial(ctx, "agent-a", Target{Service: "echo"})
			require.NoError(t, err)

			assert.Equal(t, "leak check", roundTrip(t, conn, "leak check"))
			conn.Close()
		}

		cancel()
		agent.Close()
		listener.Close()
		hub.Close()

		<-stopped
		<-served
	}()

	after := settle()

	// a few may belong to the test's own echo server, which lives until the
	// test does; anything beyond a handful is something that was not cleaned up.
	assert.LessOrEqual(t, after-before, 10, "goroutines were left running: %d before, %d after", before, after)
}
