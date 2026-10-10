package vmhost_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost/wire"
)

func TestClient_Info(t *testing.T) {
	t.Parallel()

	engine := newEngine(memory.WithCapacity(8, 16<<30, 200<<30))
	client := serve(t, engine).client

	_, err := engine.Create(t.Context(), machine("vm-1"))
	require.NoError(t, err)

	want, err := engine.Info(t.Context())
	require.NoError(t, err)

	got, err := client.Info(t.Context())
	require.NoError(t, err)

	assert.Equal(t, want, got)
	assert.Equal(t, vm.Resources{CPUs: 1, Memory: 256 << 20, Disk: 1 << 30}, got.Allocated)
}

// TestClient_Lifecycle walks a VM through everything an orchestrator does to
// one, holding what the client says at every step to what the engine behind
// the vmhost says itself.
func TestClient_Lifecycle(t *testing.T) {
	t.Parallel()

	engine := newEngine()
	client := serve(t, engine).client

	ctx := t.Context()

	same := func(t *testing.T, id string, wantState vm.InstanceState) vm.Instance {
		t.Helper()

		want, err := engine.Inspect(ctx, id)
		require.NoError(t, err)

		got, err := client.Inspect(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, want, got)
		assert.Equal(t, wantState, got.State)

		return got
	}

	created, err := client.Create(ctx, machine("vm-1"))
	require.NoError(t, err)

	assert.Equal(t, same(t, "vm-1", vm.InstanceRunning), created)
	assert.Equal(t, []vm.Endpoint{
		{Port: 80, Address: "10.89.1.10:20000"},
		{Port: 8080, Address: "10.89.1.10:20001"},
	}, created.Endpoints, "every published port says where it is reached")

	require.NoError(t, client.Stop(ctx, "vm-1"))
	same(t, "vm-1", vm.InstanceStopped)

	require.NoError(t, client.Start(ctx, "vm-1"))
	same(t, "vm-1", vm.InstanceRunning)

	require.NoError(t, client.Restart(ctx, "vm-1"))
	same(t, "vm-1", vm.InstanceRunning)

	reconfigured := machine("vm-1")
	reconfigured.Ports = []port.Port{80, 443}
	reconfigured.Resources.Memory = 512 << 20

	instance, err := client.Reconfigure(ctx, reconfigured)
	require.NoError(t, err)
	assert.Equal(t, same(t, "vm-1", vm.InstanceRunning), instance)
	assert.Equal(t, []vm.Endpoint{
		{Port: 80, Address: "10.89.1.10:20000"},
		{Port: 443, Address: "10.89.1.10:20002"},
	}, instance.Endpoints, "a port it kept keeps its host port")

	_, err = client.Create(ctx, machine("vm-2"))
	require.NoError(t, err)

	want, err := engine.List(ctx)
	require.NoError(t, err)

	got, err := client.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Len(t, got, 2)

	require.NoError(t, client.Delete(ctx, "vm-1"))
	require.NoError(t, client.Delete(ctx, "vm-1"), "deleting one that is gone is what was asked for")
	require.NoError(t, client.Delete(ctx, ""), "and so is deleting none")

	_, err = client.Inspect(ctx, "vm-1")
	assert.ErrorIs(t, err, domain.ErrNotExists)

	got, err = client.List(ctx)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

// TestClient_Spec holds a spec to reaching the engine exactly as it was
// given: an entrypoint that is not there keeps the image's own, and an empty
// one clears it, so the two may not be mistaken for each other on the way.
func TestClient_Spec(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		spec vm.Spec
	}{
		{
			name: "a docker vm",
			spec: vm.Spec{
				ID:             "vm-1",
				Image:          "docker:29-dind",
				Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
				Ports:          []port.Port{80, 443, 8080},
				Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
				PersistentDisk: true,
				Labels:         map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelOwner: "owner-uuid"},
			},
		},
		{
			name: "a code runner's task, keeping its image's entrypoint",
			spec: vm.Spec{
				ID:         "run-1",
				Image:      "python:3.12-alpine",
				Resources:  vm.Resources{CPUs: 1, Memory: 128 << 20},
				Network:    vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny},
				Labels:     map[string]string{vm.LabelPurpose: vm.PurposeTask, vm.LabelTask: "task-uuid"},
				Command:    []string{"python", "-c", "print(1)"},
				Env:        []string{"PYTHONUNBUFFERED=1"},
				WorkingDir: "/srv",
			},
		},
		{
			name: "a task that clears its image's entrypoint",
			spec: vm.Spec{
				ID:         "run-2",
				Entrypoint: []string{},
				Command:    []string{"/bin/true"},
				Ports:      []port.Port{},
				Labels:     map[string]string{},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := newEngine()
			client := serve(t, engine).client

			_, err := client.Create(t.Context(), tc.spec)
			require.NoError(t, err)

			arrived, err := engine.Spec(tc.spec.ID)
			require.NoError(t, err)

			assert.Equal(t, tc.spec, arrived)
			assert.Equal(t, tc.spec.Entrypoint == nil, arrived.Entrypoint == nil)
			assert.Equal(t, tc.spec.Ports == nil, arrived.Ports == nil)
		})
	}
}

func TestClient_StatsAndLogs(t *testing.T) {
	t.Parallel()

	engine := newEngine()
	client := serve(t, engine).client
	ctx := t.Context()

	_, err := client.Create(ctx, machine("vm-1"))
	require.NoError(t, err)

	require.NoError(t, engine.SetStats("vm-1", vm.Stats{CPUPercent: 12.5, MemoryUsed: 100 << 20, DiskUsed: 1 << 29, NetworkRx: 1 << 40, NetworkTx: 7}))

	want, err := engine.Stats(ctx, "vm-1")
	require.NoError(t, err)

	got, err := client.Stats(ctx, "vm-1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 12.5, got.CPUPercent, "a share of all of the vm's vcpus, untouched")

	for _, line := range []string{"booted", "", "hello, world", "bye"} {
		require.NoError(t, engine.Log("vm-1", vm.LogSourceMain, line))
	}

	for name, options := range map[string]vm.LogOptions{
		"all of it":           {},
		"its tail":            {Tail: 2},
		"since a moment":      {Since: at},
		"since after it all":  {Since: at.Add(time.Nanosecond)},
		"the tail of a while": {Since: at.Add(-time.Hour), Tail: 1},
	} {
		t.Run(name, func(t *testing.T) {
			want, err := engine.Logs(ctx, "vm-1", options)
			require.NoError(t, err)

			got, err := client.Logs(ctx, "vm-1", options)
			require.NoError(t, err)

			assert.Equal(t, want, got)
		})
	}

	_, err = client.Logs(ctx, "missing", vm.LogOptions{})
	assert.ErrorIs(t, err, domain.ErrNotExists)
}

// TestClient_Errors holds every error an engine answers with to being the
// same error on the orchestrator's side of the socket, in the engine's own
// words.
func TestClient_Errors(t *testing.T) {
	t.Parallel()

	foreign, err := json.Marshal(map[string]any{"engine": "another/1", "spec": map[string]any{}})
	require.NoError(t, err)

	testcases := []struct {
		name string

		// call does something to a client whose engine holds "running" and
		// "stopped", and has room for nothing more than a GiB of memory.
		call func(ctx context.Context, client *vmhost.Client) error

		want     error
		wantSaid string
	}{
		{
			name:     "inspecting a vm that is not there",
			call:     func(ctx context.Context, c *vmhost.Client) error { _, err := c.Inspect(ctx, "missing"); return err },
			want:     domain.ErrNotExists,
			wantSaid: `no instance "missing"`,
		},
		{
			name: "starting a vm that is not there",
			call: func(ctx context.Context, c *vmhost.Client) error { return c.Start(ctx, "missing") },
			want: domain.ErrNotExists,
		},
		{
			name: "reconfiguring a vm that is not there",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Reconfigure(ctx, machine("missing"))
				return err
			},
			want: domain.ErrNotExists,
		},
		{
			name: "a vm with no name",
			call: func(ctx context.Context, c *vmhost.Client) error { _, err := c.Stats(ctx, ""); return err },
			want: domain.ErrNotExists,
		},
		{
			name: "creating a vm that is there already",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Create(ctx, machine("running"))
				return err
			},
			want:     domain.ErrAlreadyExists,
			wantSaid: `"running"`,
		},
		{
			name: "creating a vm with no name",
			call: func(ctx context.Context, c *vmhost.Client) error { _, err := c.Create(ctx, vm.Spec{}); return err },
			want: wire.ErrInvalid,
		},
		{
			name: "creating a vm the node has no room for",
			call: func(ctx context.Context, c *vmhost.Client) error {
				big := machine("big")
				big.Resources.Memory = 1 << 30

				_, err := c.Create(ctx, big)
				return err
			},
			want:     vm.ErrNoCapacity,
			wantSaid: "bytes of memory asked",
		},
		{
			name: "growing a vm past the node's room",
			call: func(ctx context.Context, c *vmhost.Client) error {
				bigger := machine("running")
				bigger.Resources.Memory = 1 << 30

				_, err := c.Reconfigure(ctx, bigger)
				return err
			},
			want: vm.ErrNoCapacity,
		},
		{
			name: "sampling a vm that is not running",
			call: func(ctx context.Context, c *vmhost.Client) error { _, err := c.Stats(ctx, "stopped"); return err },
			want: vm.ErrNotRunning,
		},
		{
			name: "a command in a vm that is not running",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Exec(ctx, "stopped", vm.ExecOptions{Command: []string{"true"}})
				return err
			},
			want:     vm.ErrNotRunning,
			wantSaid: `"stopped" is stopped`,
		},
		{
			name: "a command in a vm that is not there",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Exec(ctx, "missing", vm.ExecOptions{Command: []string{"true"}})
				return err
			},
			want: domain.ErrNotExists,
		},
		{
			name: "a snapshot of a vm that is not there",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Snapshot(ctx, "missing", &strings.Builder{})
				return err
			},
			want: domain.ErrNotExists,
		},
		{
			name: "restoring an archive another engine wrote",
			call: func(ctx context.Context, c *vmhost.Client) error {
				_, err := c.Restore(ctx, machine("restored"), strings.NewReader(string(foreign)))
				return err
			},
			want:     vm.ErrEngineMismatch,
			wantSaid: `"another/1"`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			engine := newEngine(memory.WithCapacity(0, 1<<30, 0))
			client := serve(t, engine).client

			_, err := engine.Create(t.Context(), machine("running"))
			require.NoError(t, err)

			_, err = engine.Create(t.Context(), machine("stopped"))
			require.NoError(t, err)
			require.NoError(t, engine.Stop(t.Context(), "stopped"))

			err = tc.call(t.Context(), client)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.want)
			assert.Contains(t, err.Error(), tc.wantSaid)
		})
	}
}

func TestClient_Unavailable(t *testing.T) {
	t.Parallel()

	t.Run("no socket at all", func(t *testing.T) {
		t.Parallel()

		socket := socketPath(t)
		client := vmhost.NewClient(socket)

		_, err := client.Info(t.Context())

		assert.ErrorIs(t, err, vmhost.ErrUnavailable)
		assert.Contains(t, err.Error(), socket, "it says which vmhost is not answering")

		_, err = client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"true"}})
		assert.ErrorIs(t, err, vmhost.ErrUnavailable)
	})

	t.Run("a socket nobody listens on", func(t *testing.T) {
		t.Parallel()

		socket := socketPath(t)

		listener, err := net.Listen("unix", socket)
		require.NoError(t, err)

		listener.(*net.UnixListener).SetUnlinkOnClose(false)
		require.NoError(t, listener.Close())

		err = vmhost.NewClient(socket).Start(t.Context(), "vm-1")
		assert.ErrorIs(t, err, vmhost.ErrUnavailable)
	})

	t.Run("a vmhost shutting down takes no new sessions", func(t *testing.T) {
		t.Parallel()

		engine := newEngine()
		h := serve(t, engine)

		_, err := engine.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		require.NoError(t, h.server.Close())

		_, err = h.client.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"true"}})
		assert.ErrorIs(t, err, vmhost.ErrUnavailable)
	})
}

// traced is an engine that keeps the trace each Info was asked in.
type traced struct {
	vm.Engine

	lock   sync.Mutex
	traces []trace.TraceID
}

func (e *traced) Info(ctx context.Context) (vm.Info, error) {
	e.lock.Lock()
	e.traces = append(e.traces, trace.SpanContextFromContext(ctx).TraceID())
	e.lock.Unlock()

	return e.Engine.Info(ctx)
}

// TestClient_Trace holds a call to staying in the trace it was made in, across
// the socket: what the engine does for it is part of the same trace.
func TestClient_Trace(t *testing.T) {
	t.Parallel()

	engine := &traced{Engine: newEngine()}
	client := serve(t, engine).client

	traceID := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     trace.SpanID{0, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	_, err := client.Info(trace.ContextWithRemoteSpanContext(t.Context(), parent))
	require.NoError(t, err)

	engine.lock.Lock()
	defer engine.lock.Unlock()

	assert.Equal(t, []trace.TraceID{traceID}, engine.traces)
}

// blocking is an engine whose Info and Create wait to be let go, and say what
// became of the context they were given.
type blocking struct {
	vm.Engine

	release chan struct{}
	asked   chan context.Context
	done    chan error
}

func newBlocking(engine vm.Engine) *blocking {
	return &blocking{
		Engine:  engine,
		release: make(chan struct{}),
		asked:   make(chan context.Context, 1),
		done:    make(chan error, 1),
	}
}

func (e *blocking) Info(ctx context.Context) (vm.Info, error) {
	e.asked <- ctx

	select {
	case <-ctx.Done():
		e.done <- ctx.Err()

		return vm.Info{}, ctx.Err()
	case <-e.release:
		e.done <- nil

		return e.Engine.Info(ctx)
	}
}

func (e *blocking) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	e.asked <- ctx
	<-e.release

	instance, err := e.Engine.Create(ctx, spec)
	e.done <- err

	return instance, err
}

func TestClient_Cancellation(t *testing.T) {
	t.Parallel()

	t.Run("a caller that gives up on a question stops the vmhost asking it", func(t *testing.T) {
		t.Parallel()

		engine := newBlocking(newEngine())
		client := serve(t, engine).client

		ctx, cancel := context.WithCancel(t.Context())

		failed := make(chan error, 1)
		go func() {
			_, err := client.Info(ctx)
			failed <- err
		}()

		<-engine.asked
		cancel()

		select {
		case err := <-failed:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(settle):
			t.Fatal("a call its caller gave up on went on being waited for")
		}

		select {
		case err := <-engine.done:
			assert.ErrorIs(t, err, context.Canceled, "the engine is told its asker is gone")
		case <-time.After(settle):
			t.Fatal("the engine was left answering a question nobody waits for")
		}
	})

	t.Run("a change the vmhost has begun is carried through", func(t *testing.T) {
		t.Parallel()

		engine := newBlocking(newEngine())
		client := serve(t, engine).client

		ctx, cancel := context.WithCancel(t.Context())

		failed := make(chan error, 1)
		go func() {
			_, err := client.Create(ctx, machine("vm-1"))
			failed <- err
		}()

		engineCtx := <-engine.asked
		cancel()

		assert.ErrorIs(t, <-failed, context.Canceled)

		// a create stopped halfway leaves a vm halfway, so the engine is not
		// told its asker gave up: what it makes is in the next heartbeat.
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, engineCtx.Err())

		close(engine.release)
		require.NoError(t, <-engine.done)

		instance, err := client.Inspect(t.Context(), "vm-1")
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, instance.State)
	})

	t.Run("a call asked with a context already done is not made", func(t *testing.T) {
		t.Parallel()

		engine := newEngine()
		client := serve(t, engine).client

		_, err := engine.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err = client.List(ctx)
		assert.ErrorIs(t, err, context.Canceled)

		_, err = client.Exec(ctx, "vm-1", vm.ExecOptions{Command: []string{"true"}})
		assert.ErrorIs(t, err, context.Canceled)

		assert.Zero(t, engine.Sessions("vm-1"), "no command was left running")
	})

	t.Run("a vmhost that never answers is given up on", func(t *testing.T) {
		t.Parallel()

		// a listener that takes connections and says nothing on them.
		socket := socketPath(t)
		listener, err := net.Listen("unix", socket)
		require.NoError(t, err)

		accepted := make(chan net.Conn, 8)
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					close(accepted)

					return
				}

				accepted <- conn
			}
		}()

		t.Cleanup(func() {
			_ = listener.Close()

			for conn := range accepted {
				_ = conn.Close()
			}
		})

		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		_, err = vmhost.NewClient(socket).Exec(ctx, "vm-1", vm.ExecOptions{Command: []string{"true"}})
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "the handshake is given up on with its caller: %v", err)
	})
}
