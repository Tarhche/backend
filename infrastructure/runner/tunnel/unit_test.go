package tunnel

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Run("the defaults are usable", func(t *testing.T) {
		assert.NoError(t, DefaultConfig().Validate())
	})

	t.Run("what cannot work is refused", func(t *testing.T) {
		tests := []struct {
			name  string
			spoil func(*Config)
		}{
			{name: "no sessions at all", spoil: func(c *Config) { c.MinSessions = 0 }},
			{name: "fewer most than fewest", spoil: func(c *Config) { c.MaxSessions = c.MinSessions - 1 }},
			{name: "no streams", spoil: func(c *Config) { c.MaxStreamsPerSession = 0 }},
			{name: "a threshold of nothing", spoil: func(c *Config) { c.GrowThreshold = 0 }},
			{name: "a threshold past everything", spoil: func(c *Config) { c.GrowThreshold = 1.5 }},
			{name: "a stream allowed more than the session", spoil: func(c *Config) { c.MaxStreamBuffer = c.MaxReceiveBuffer + 1 }},
			{name: "a frame larger than the window", spoil: func(c *Config) { c.MaxFrameSize = c.MaxStreamBuffer + 1 }},
			{name: "a version smux does not speak", spoil: func(c *Config) { c.Version = 3 }},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				tt.spoil(&config)

				assert.Error(t, config.Validate())
			})
		}
	})

	t.Run("capacity is what the pool can carry at its largest", func(t *testing.T) {
		config := DefaultConfig()

		assert.Equal(t, config.MaxSessions*config.MaxStreamsPerSession, config.Capacity())
	})

	t.Run("smux is given exactly what it owns", func(t *testing.T) {
		config := DefaultConfig()
		smuxConfig := config.smux()

		assert.Equal(t, 2, smuxConfig.Version, "version 2 is what gives a stream its own window")
		assert.Equal(t, config.KeepAliveInterval, smuxConfig.KeepAliveInterval)
		assert.Equal(t, config.KeepAliveTimeout, smuxConfig.KeepAliveTimeout)
		assert.Equal(t, config.MaxFrameSize, smuxConfig.MaxFrameSize)
		assert.Equal(t, config.MaxReceiveBuffer, smuxConfig.MaxReceiveBuffer)
		assert.Equal(t, config.MaxStreamBuffer, smuxConfig.MaxStreamBuffer)
	})

	t.Run("the keepalive timeout is a multiple of the interval", func(t *testing.T) {
		config := DefaultConfig()

		// one lost keepalive must not kill a healthy session
		assert.GreaterOrEqual(t, config.KeepAliveTimeout, 3*config.KeepAliveInterval)
	})
}

func TestTarget(t *testing.T) {
	t.Run("a service is named", func(t *testing.T) {
		target := Target{Service: "api"}

		assert.True(t, target.Named())
		assert.True(t, target.Valid())
		assert.Equal(t, "api", target.String())
	})

	t.Run("an address is not", func(t *testing.T) {
		target := Target{Host: "127.0.0.1", Port: 22}

		assert.False(t, target.Named())
		assert.True(t, target.Valid())
		assert.Equal(t, "127.0.0.1:22", target.String())
	})

	t.Run("nothing at all is not a target", func(t *testing.T) {
		assert.False(t, Target{}.Valid())
		assert.False(t, Target{Host: "127.0.0.1"}.Valid())
		assert.False(t, Target{Port: 22}.Valid())
	})
}

func TestServiceTargets(t *testing.T) {
	targets := NewServiceTargets(
		map[string]string{"api": "127.0.0.1:8080"},
		AddressRule{Host: "127.0.0.1", Ports: []uint16{22}},
		AddressRule{Host: "10.0.0.5", From: 30000, To: 30100},
	)

	t.Run("a service it offers", func(t *testing.T) {
		address, err := targets.Resolve(t.Context(), Target{Service: "api"})

		assert.NoError(t, err)
		assert.Equal(t, "127.0.0.1:8080", address)
	})

	t.Run("a service it does not", func(t *testing.T) {
		_, err := targets.Resolve(t.Context(), Target{Service: "database"})

		assert.ErrorIs(t, err, ErrUnknownService)
	})

	t.Run("an address it allows", func(t *testing.T) {
		address, err := targets.Resolve(t.Context(), Target{Host: "127.0.0.1", Port: 22})

		assert.NoError(t, err)
		assert.Equal(t, "127.0.0.1:22", address)
	})

	t.Run("a port within an allowed span", func(t *testing.T) {
		address, err := targets.Resolve(t.Context(), Target{Host: "10.0.0.5", Port: 30050})

		assert.NoError(t, err)
		assert.Equal(t, "10.0.0.5:30050", address)
	})

	t.Run("a port outside it", func(t *testing.T) {
		_, err := targets.Resolve(t.Context(), Target{Host: "10.0.0.5", Port: 30101})

		assert.ErrorIs(t, err, ErrTargetNotAllowed)
	})

	t.Run("a host it was never told about", func(t *testing.T) {
		_, err := targets.Resolve(t.Context(), Target{Host: "192.168.1.1", Port: 22})

		assert.ErrorIs(t, err, ErrTargetNotAllowed)
	})

	t.Run("nothing at all", func(t *testing.T) {
		_, err := targets.Resolve(t.Context(), Target{})

		assert.ErrorIs(t, err, ErrTargetNotAllowed)
	})

	t.Run("a worker that offers only names will dial nothing else", func(t *testing.T) {
		only := NewServiceTargets(map[string]string{"api": "127.0.0.1:8080"})

		_, err := only.Resolve(t.Context(), Target{Host: "127.0.0.1", Port: 8080})

		assert.ErrorIs(t, err, ErrTargetNotAllowed)
	})

	t.Run("what is offered can change while it runs", func(t *testing.T) {
		changing := NewServiceTargets(nil)

		_, err := changing.Resolve(t.Context(), Target{Service: "later"})
		assert.ErrorIs(t, err, ErrUnknownService)

		changing.Set("later", "127.0.0.1:9999")

		address, err := changing.Resolve(t.Context(), Target{Service: "later"})
		assert.NoError(t, err)
		assert.Equal(t, "127.0.0.1:9999", address)

		changing.Remove("later")

		_, err = changing.Resolve(t.Context(), Target{Service: "later"})
		assert.ErrorIs(t, err, ErrUnknownService)
	})
}

func TestLeastLoadedRouter(t *testing.T) {
	router := LeastLoaded()

	t.Run("the emptiest is picked", func(t *testing.T) {
		worker, err := router.Pick([]WorkerState{
			{Worker: "busy", Sessions: 1, Streams: 8, Capacity: 10},
			{Worker: "quiet", Sessions: 1, Streams: 1, Capacity: 10},
		})

		assert.NoError(t, err)
		assert.Equal(t, "quiet", worker)
	})

	t.Run("load is a share rather than a count, so unequal workers compare", func(t *testing.T) {
		worker, err := router.Pick([]WorkerState{
			{Worker: "small", Sessions: 1, Streams: 5, Capacity: 10},
			{Worker: "large", Sessions: 4, Streams: 20, Capacity: 100},
		})

		assert.NoError(t, err)
		assert.Equal(t, "large", worker)
	})

	t.Run("a full worker is not picked", func(t *testing.T) {
		worker, err := router.Pick([]WorkerState{
			{Worker: "full", Sessions: 1, Streams: 10, Capacity: 10},
			{Worker: "room", Sessions: 1, Streams: 9, Capacity: 10},
		})

		assert.NoError(t, err)
		assert.Equal(t, "room", worker)
	})

	t.Run("a worker with nothing connected is not picked", func(t *testing.T) {
		_, err := router.Pick([]WorkerState{{Worker: "gone", Sessions: 0, Capacity: 10}})

		assert.ErrorIs(t, err, ErrNoWorkerAvailable)
	})

	t.Run("nobody at all", func(t *testing.T) {
		_, err := router.Pick(nil)

		assert.ErrorIs(t, err, ErrNoWorkerAvailable)
	})

	t.Run("everybody full", func(t *testing.T) {
		_, err := router.Pick([]WorkerState{{Worker: "full", Sessions: 1, Streams: 10, Capacity: 10}})

		assert.ErrorIs(t, err, ErrNoWorkerAvailable)
	})
}

func TestWorkerState(t *testing.T) {
	t.Run("a worker with no capacity is fully loaded rather than empty", func(t *testing.T) {
		state := WorkerState{Worker: "gone"}

		assert.Equal(t, float64(1), state.Load())
		assert.Zero(t, state.Free())
	})

	t.Run("free never goes below nothing", func(t *testing.T) {
		state := WorkerState{Streams: 12, Capacity: 10}

		assert.Zero(t, state.Free())
	})
}

func TestRegistry(t *testing.T) {
	t.Run("a session makes its worker exist, and the last one leaving unmakes it", func(t *testing.T) {
		registry := NewRegistry()

		first := fakeSession(t, "worker-a", 4)
		second := fakeSession(t, "worker-a", 4)

		require.NoError(t, registry.Add(first))
		require.NoError(t, registry.Add(second))

		sessions, err := registry.Sessions("worker-a")
		require.NoError(t, err)
		assert.Len(t, sessions, 2)

		registry.Remove(first)

		sessions, err = registry.Sessions("worker-a")
		require.NoError(t, err)
		assert.Len(t, sessions, 1)

		registry.Remove(second)

		_, err = registry.Sessions("worker-a")
		assert.ErrorIs(t, err, ErrNoSuchWorker)
		assert.Empty(t, registry.Workers())
	})

	t.Run("a session belonging to nobody is refused", func(t *testing.T) {
		assert.Error(t, NewRegistry().Add(fakeSession(t, "", 4)))
	})

	t.Run("removing what was never there is not an error", func(t *testing.T) {
		registry := NewRegistry()

		registry.Remove(fakeSession(t, "worker-a", 4))
	})

	t.Run("workers come back ordered, with what they are carrying", func(t *testing.T) {
		registry := NewRegistry()

		for _, name := range []string{"worker-c", "worker-a", "worker-b"} {
			require.NoError(t, registry.Add(fakeSession(t, name, 4)))
		}

		states := registry.Workers()
		require.Len(t, states, 3)

		assert.Equal(t, "worker-a", states[0].Worker)
		assert.Equal(t, "worker-b", states[1].Worker)
		assert.Equal(t, "worker-c", states[2].Worker)
		assert.Equal(t, 4, states[0].Capacity)
	})

	t.Run("adding while the last one is removed does not lose the new one", func(t *testing.T) {
		registry := NewRegistry()

		// the sessions are made up front: building one is not what is being
		// tested, and a test helper is not safe to call from a goroutine.
		sessions := make([]*Session, 50)
		for i := range sessions {
			sessions[i] = fakeSession(t, "worker-a", 4)
		}

		var wait sync.WaitGroup
		for i, session := range sessions {
			wait.Add(1)

			go func() {
				defer wait.Done()

				if err := registry.Add(session); err != nil {
					return
				}

				_, _ = registry.Sessions("worker-a")
				registry.Workers()

				// half of them leave again, so the worker is repeatedly taken
				// down to nothing while others are still arriving
				if i%2 == 0 {
					registry.Remove(session)
				}
			}()
		}

		wait.Wait()

		held, err := registry.Sessions("worker-a")
		require.NoError(t, err, "the worker should still be there")
		assert.Len(t, held, 25, "a session added while the last one left was dropped")
	})
}

func TestSessionCapacity(t *testing.T) {
	t.Run("places are handed out until there are none", func(t *testing.T) {
		session := fakeSession(t, "worker-a", 2)

		assert.Equal(t, 2, session.Free())
		assert.True(t, session.reserve())
		assert.True(t, session.reserve())
		assert.False(t, session.reserve(), "a third should not fit in two places")

		assert.Equal(t, 2, session.Streams())
		assert.Zero(t, session.Free())
		assert.False(t, session.Usable())

		session.release()

		assert.Equal(t, 1, session.Streams())
		assert.True(t, session.Usable())
	})

	t.Run("concurrent reservations never overshoot", func(t *testing.T) {
		session := fakeSession(t, "worker-a", 10)

		var (
			wait   sync.WaitGroup
			taken  = make(chan struct{}, 100)
			failed = make(chan struct{}, 100)
		)

		for range 100 {
			wait.Add(1)

			go func() {
				defer wait.Done()

				if session.reserve() {
					taken <- struct{}{}
				} else {
					failed <- struct{}{}
				}
			}()
		}

		wait.Wait()

		assert.Len(t, taken, 10, "exactly the capacity should have been handed out")
		assert.Len(t, failed, 90)
		assert.Equal(t, 10, session.Streams())
	})
}

func TestStreamProxy(t *testing.T) {
	t.Run("bytes go both ways and the counts are reported", func(t *testing.T) {
		leftEnd, left := net.Pipe()
		rightEnd, right := net.Pipe()

		done := make(chan struct{})
		var sent, received int64

		go func() {
			defer close(done)

			sent, received, _ = StreamProxy{}.Copy(left, right)
		}()

		go func() {
			buffer := make([]byte, 5)
			_, _ = io.ReadFull(rightEnd, buffer)
			_, _ = rightEnd.Write([]byte("world!"))
			rightEnd.Close()
		}()

		_, err := io.WriteString(leftEnd, "hello")
		require.NoError(t, err)

		answer, err := io.ReadAll(leftEnd)
		require.NoError(t, err)
		assert.Equal(t, "world!", string(answer))

		leftEnd.Close()
		<-done

		assert.EqualValues(t, 5, sent)
		assert.EqualValues(t, 6, received)
	})

	t.Run("closing one end finishes the copy rather than hanging", func(t *testing.T) {
		leftEnd, left := net.Pipe()
		_, right := net.Pipe()

		done := make(chan struct{})
		go func() {
			defer close(done)

			_, _, _ = StreamProxy{}.Copy(left, right)
		}()

		leftEnd.Close()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the copy did not finish when a side was closed")
		}
	})
}

func TestPrefixedConn(t *testing.T) {
	t.Run("what was read early is read again first", func(t *testing.T) {
		end, conn := net.Pipe()

		go func() {
			_, _ = io.WriteString(end, "rest")
			end.Close()
		}()

		prefixed := withPrefix(conn, []byte("head "))

		all, err := io.ReadAll(prefixed)
		require.NoError(t, err)
		assert.Equal(t, "head rest", string(all))
	})

	t.Run("nothing read early is the connection itself", func(t *testing.T) {
		_, conn := net.Pipe()

		assert.Equal(t, conn, withPrefix(conn, nil))
	})
}

func TestBackoff(t *testing.T) {
	worker := &Worker{config: testConfig(), closed: make(chan struct{})}
	pool := newSessionPool(worker, "127.0.0.1:1")

	t.Run("it grows, and never past the ceiling", func(t *testing.T) {
		seen := make([]time.Duration, 0, 20)

		for i := range 20 {
			pool.attempt = i + 1

			delay := pool.backoff()

			assert.Positive(t, delay)
			assert.LessOrEqual(t, delay, worker.config.ReconnectMaxDelay)

			seen = append(seen, delay)
		}

		assert.NotEmpty(t, seen)
	})

	t.Run("two waits are not the same, so workers do not come back in step", func(t *testing.T) {
		pool.attempt = 8

		distinct := make(map[time.Duration]struct{})
		for range 50 {
			distinct[pool.backoff()] = struct{}{}
		}

		assert.Greater(t, len(distinct), 1, "the delay should be jittered, not fixed")
	})
}

func TestCounters(t *testing.T) {
	counters := &Counters{}

	counters.SessionOpened("worker-a", "s1")
	counters.StreamOpened("worker-a", "s1", "api")
	counters.StreamClosed("worker-a", "s1", "api", 100, 200)
	counters.StreamFailed("worker-a", "api", "target")
	counters.AuthenticationFailed("worker-a", "token")
	counters.Reconnected("worker-a", "ingress:81", 2)
	counters.SessionClosed("worker-a", "s1", "closed")

	assert.EqualValues(t, 1, counters.SessionsOpened.Load())
	assert.EqualValues(t, 1, counters.SessionsClosed.Load())
	assert.EqualValues(t, 1, counters.StreamsOpened.Load())
	assert.EqualValues(t, 1, counters.StreamsClosed.Load())
	assert.EqualValues(t, 1, counters.StreamsFailed.Load())
	assert.EqualValues(t, 1, counters.AuthFailures.Load())
	assert.EqualValues(t, 1, counters.Reconnects.Load())
	assert.EqualValues(t, 100, counters.BytesSent.Load())
	assert.EqualValues(t, 200, counters.BytesReceived.Load())
}

func TestIngressRequiresAuthenticator(t *testing.T) {
	_, err := NewIngress(DefaultConfig(), nil, discardLogger())

	assert.Error(t, err, "a tunnel that takes anything is a way into every worker behind it")
}

func TestWorkerRequires(t *testing.T) {
	config := testConfig()
	targets := NewServiceTargets(nil)

	tests := []struct {
		name      string
		id        string
		addresses []string
		dialer    Dialer
		targets   Targets
	}{
		{name: "a name", addresses: []string{"x:1"}, dialer: plainDialer(), targets: targets},
		{name: "an ingress", id: "worker-a", dialer: plainDialer(), targets: targets},
		{name: "a dialer", id: "worker-a", addresses: []string{"x:1"}, targets: targets},
		{name: "somewhere it may connect", id: "worker-a", addresses: []string{"x:1"}, dialer: plainDialer()},
	}

	for _, tt := range tests {
		t.Run("it needs "+tt.name, func(t *testing.T) {
			_, err := NewWorker(tt.id, tt.addresses, config, tt.dialer, tt.targets, discardLogger())

			assert.Error(t, err)
		})
	}
}

// fakeSession is a session with no connection under it, for the parts that are
// about counting rather than about carrying anything.
func fakeSession(t *testing.T, worker string, capacity int) *Session {
	t.Helper()

	left, right := net.Pipe()
	t.Cleanup(func() {
		left.Close()
		right.Close()
	})

	muxSession, err := smuxServerOn(right)
	require.NoError(t, err)

	t.Cleanup(func() { muxSession.Close() })

	return newSession(newID(), worker, muxSession, capacity)
}

func TestIDsAreDistinct(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := newID()

		_, repeated := seen[id]
		assert.False(t, repeated, "two sessions would be indistinguishable in a log")

		seen[id] = struct{}{}
	}
}
