package client

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

func TestRefuse(t *testing.T) {
	t.Parallel()

	stack := network.Attachments(network.PolicyIsolated, "shop", "api")
	isolated := network.Attachments(network.PolicyIsolated, "", "")
	none := network.Attachments(network.PolicyNone, "", "")

	tests := []struct {
		name      string
		execution task.Execution
		want      error
	}{
		{name: "an isolated task runs", execution: task.Execution{Networks: isolated}},
		{name: "so does a public one", execution: task.Execution{Networks: network.Attachments(network.PolicyPublic, "", "")}},
		{name: "a stack's service does not", execution: task.Execution{Networks: stack}, want: ErrStack},
		{name: "nor a read-only task", execution: task.Execution{Networks: isolated, ReadOnly: true}, want: ErrReadOnlyRoot},
		{name: "nor one on docker's none", execution: task.Execution{Networks: none}, want: ErrNoNetwork},
		{name: "nor one on none among others", execution: task.Execution{Networks: append(isolated, none...)}, want: ErrNoNetwork},
		{name: "nor one on nothing at all", execution: task.Execution{}, want: ErrNoNetwork},

		// what is wrong first is what the task is told.
		{name: "a read-only service of a stack is a stack first", execution: task.Execution{Networks: stack, ReadOnly: true}, want: ErrStack},
		{name: "a read-only task with no network is read-only first", execution: task.Execution{Networks: none, ReadOnly: true}, want: ErrReadOnlyRoot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, refuse(&tt.execution))
		})
	}
}

func TestPolicyOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, api.NetworkPublic, policyOf(network.Attachments(network.PolicyPublic, "", "")))
	assert.Equal(t, api.NetworkIsolated, policyOf(network.Attachments(network.PolicyIsolated, "", "")))
	assert.Equal(t, api.NetworkPublic, policyOf([]network.Attachment{{Name: "elsewhere", Gateway: true}}), "the network a task routes out through is a public one")
	assert.Equal(t, api.NetworkIsolated, policyOf([]network.Attachment{{Name: "elsewhere"}}), "a network nobody said routes out is isolated")

	// and back again, as an inspected run reports what it was asked for.
	for _, asked := range []network.Policy{network.PolicyPublic, network.PolicyIsolated} {
		attachments := network.Attachments(asked, "", "")
		assert.Equal(t, attachments, network.Attachments(policy(policyOf(attachments)), "", ""))
	}
}

func TestPublishedPorts(t *testing.T) {
	t.Parallel()

	t.Run("every port exposed or bound, each once, in order", func(t *testing.T) {
		t.Parallel()

		ports, err := publishedPorts(&task.Execution{
			ExposedPorts: port.PortSet{8080: {}, 80: {}},
			PortBindings: port.PortMap{443: {{HostIP: "0.0.0.0"}}, 80: {{HostIP: "0.0.0.0", HostPort: 8000}}},
		})
		require.NoError(t, err)

		assert.Equal(t, []uint16{80, 443, 8080}, ports)
	})

	t.Run("none is none", func(t *testing.T) {
		t.Parallel()

		ports, err := publishedPorts(&task.Execution{ExposedPorts: port.PortSet{}, PortBindings: port.PortMap{}})
		require.NoError(t, err)

		assert.Nil(t, ports)
	})

	t.Run("a port TCP does not have is refused rather than wrapped round", func(t *testing.T) {
		t.Parallel()

		for _, p := range []port.Port{0, math.MaxUint16 + 1, math.MaxUint16 + 80} {
			_, err := publishedPorts(&task.Execution{ExposedPorts: port.PortSet{p: {}}})

			assert.Error(t, err, "%d", p)
		}

		ports, err := publishedPorts(&task.Execution{ExposedPorts: port.PortSet{math.MaxUint16: {}, 1: {}}})
		require.NoError(t, err)
		assert.Equal(t, []uint16{1, math.MaxUint16}, ports)
	})
}

func TestTTLSeconds(t *testing.T) {
	t.Parallel()

	tests := map[time.Duration]int64{
		0:                       0,
		-time.Second:            0,
		time.Nanosecond:         1,
		500 * time.Millisecond:  1,
		time.Second:             1,
		1500 * time.Millisecond: 2,
		time.Hour:               3600,
	}

	for ttl, want := range tests {
		assert.Equal(t, want, ttlSeconds(ttl), "%s", ttl)
	}
}

func TestRunSpec(t *testing.T) {
	t.Parallel()

	t.Run("a run belongs to the node the execution names", func(t *testing.T) {
		t.Parallel()

		spec, err := runSpec(&task.Execution{NodeName: "workload-orchestrator-02", Networks: network.Attachments(network.PolicyIsolated, "", "")}, "workload-orchestrator-01")
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-02", spec.Node)
	})

	t.Run("or to the client's own, when it names none, rather than to nobody", func(t *testing.T) {
		t.Parallel()

		spec, err := runSpec(&task.Execution{Networks: network.Attachments(network.PolicyIsolated, "", "")}, "workload-orchestrator-01")
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-01", spec.Node)
	})

	t.Run("a task that names no kind is the default kind, as docker's labels read back", func(t *testing.T) {
		t.Parallel()

		spec, err := runSpec(&task.Execution{Networks: network.Attachments(network.PolicyIsolated, "", "")}, "workload-orchestrator-01")
		require.NoError(t, err)

		assert.Equal(t, string(task.DefaultKind), spec.Task.Kind)
	})
}

func TestExecution(t *testing.T) {
	t.Parallel()

	t.Run("a run's state is the status a container would have", func(t *testing.T) {
		t.Parallel()

		tests := map[api.State]task.Status{
			api.StateCreated:    task.StatusCreated,
			api.StateStarting:   task.StatusCreated,
			api.StateRunning:    task.StatusRunning,
			api.StateStopping:   task.StatusRunning,
			api.StateRestarting: task.StatusRestarting,
			api.StateExited:     task.StatusExited,
			"hibernating":       0,
		}

		for state, want := range tests {
			got := execution(&api.Run{State: state})

			assert.Equal(t, want, got.Status, string(state))
		}
	})

	t.Run("ports are reported while the run runs, and only then", func(t *testing.T) {
		t.Parallel()

		run := api.Run{
			RunSpec:   api.RunSpec{Ports: []uint16{80, 443, 9000}},
			Endpoints: []api.Endpoint{{Port: 80, HostPort: 20000}, {Port: 443, HostPort: 20001}, {Port: 9000}},
		}

		for _, state := range []api.State{api.StateRunning, api.StateStopping} {
			run.State = state
			got := execution(&run)

			assert.Equal(t, port.PortSet{80: {}, 443: {}, 9000: {}}, got.ExposedPorts, string(state))
			assert.Equal(t, port.PortMap{
				80:  {{HostIP: "0.0.0.0", HostPort: 20000}},
				443: {{HostIP: "0.0.0.0", HostPort: 20001}},
			}, got.PortBindings, "a port with no host port is not published: %s", state)
		}

		for _, state := range []api.State{api.StateCreated, api.StateStarting, api.StateRestarting, api.StateExited} {
			run.State = state
			got := execution(&run)

			assert.Equal(t, port.PortSet{}, got.ExposedPorts, string(state))
			assert.Equal(t, port.PortMap{}, got.PortBindings, string(state))
		}
	})

	t.Run("what the service cannot have meant is read as docker's labels are", func(t *testing.T) {
		t.Parallel()

		got := execution(&api.Run{Task: api.Task{Kind: "cronjob", Attempt: -1, TTLSeconds: -60}})

		assert.Equal(t, task.DefaultKind, got.Kind)
		assert.Zero(t, got.Attempt)
		assert.Zero(t, got.TTL)
	})
}

func TestNewestFirst(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.October, 4, 9, 30, 0, 0, time.UTC)

	executions := []task.Execution{
		{ID: "0001", CreatedAt: at},
		{ID: "0003", CreatedAt: at.Add(time.Second)},
		{ID: "0002", CreatedAt: at.Add(time.Second)},
		{ID: "0004", CreatedAt: at.Add(-time.Second)},
	}

	slices.SortStableFunc(executions, newestFirst)

	ids := make([]string, len(executions))
	for i, e := range executions {
		ids[i] = e.ID
	}

	assert.Equal(t, []string{"0003", "0002", "0001", "0004"}, ids)
}

func TestStats(t *testing.T) {
	t.Parallel()

	assert.Equal(t, task.Stats{MemoryUsage: 64 << 20}, taskStats(api.Stats{MemoryUsage: 64 << 20}), "no limit is no share of one")
	assert.InDelta(t, 12.5, taskStats(api.Stats{MemoryUsage: 32 << 20, MemoryLimit: 256 << 20}).MemoryPercent, 1e-9)
	assert.InDelta(t, 50.0, nodeStats(api.Stats{MemoryUsage: 1 << 30, MemoryLimit: 2 << 30}).MemoryPercent, 1e-9)
}

func TestStream(t *testing.T) {
	t.Parallel()

	assert.Equal(t, task.StreamStdout, stream(api.StreamStdout))
	assert.Equal(t, task.StreamStderr, stream(api.StreamStderr))
	assert.Equal(t, task.StreamStdout, stream("console"), "a line from a stream nobody knows is still output")
}

func TestDimension(t *testing.T) {
	t.Parallel()

	assert.Equal(t, uint16(40), dimension(40))
	assert.Equal(t, uint16(math.MaxUint16), dimension(math.MaxUint16+1))
}

func TestServiceURL(t *testing.T) {
	t.Parallel()

	reached := map[string]string{
		"https://workload-microsandbox:8443":  "https://workload-microsandbox:8443",
		"https://workload-microsandbox:8443/": "https://workload-microsandbox:8443",
		"https://10.89.0.10:8443":             "https://10.89.0.10:8443",
		"https://[fd00::10]:8443":             "https://[fd00::10]:8443",
		"https://workload-microsandbox":       "https://workload-microsandbox",
	}

	for raw, want := range reached {
		got, err := serviceURL(raw)
		require.NoError(t, err, raw)

		assert.Equal(t, want, got.String())
	}

	notReached := []string{
		"",
		"workload-microsandbox:8443",
		"http://workload-microsandbox:8443",
		"wss://workload-microsandbox:8443",
		"https://:8443",
		"https://workload-microsandbox:8443/v1",
		"https://workload-microsandbox:8443?node=a",
		"https://workload-microsandbox:8443#v1",
		"https://user:secret@workload-microsandbox:8443",
		"https://workload microsandbox:8443",
	}

	for _, raw := range notReached {
		_, err := serviceURL(raw)

		assert.ErrorIs(t, err, ErrNotAService, raw)
	}
}

func TestLocate(t *testing.T) {
	t.Parallel()

	base, err := serviceURL("https://workload-microsandbox:8443")
	require.NoError(t, err)

	c := &Client{base: base}

	t.Run("a route is asked for with its wildcards filled in", func(t *testing.T) {
		t.Parallel()

		method, location, err := c.locate(api.RouteEndExec, map[string]string{api.WildcardRun: "01926f3a", api.WildcardExec: "exec-1"})
		require.NoError(t, err)

		assert.Equal(t, "POST", method)
		assert.Equal(t, "https://workload-microsandbox:8443/v1/runs/01926f3a/execs/exec-1/end", location.String())
	})

	t.Run("a wildcard is one segment, whatever it holds", func(t *testing.T) {
		t.Parallel()

		_, location, err := c.locate(api.RouteGetRun, runPath("a/b c"))
		require.NoError(t, err)

		assert.Equal(t, "https://workload-microsandbox:8443/v1/runs/a%2Fb%20c", location.String())
	})

	t.Run("a route with nothing to fill in is itself", func(t *testing.T) {
		t.Parallel()

		method, location, err := c.locate(api.RouteListRuns, nil)
		require.NoError(t, err)

		assert.Equal(t, "GET", method)
		assert.Equal(t, "https://workload-microsandbox:8443/v1/runs", location.String())
	})

	t.Run("a wildcard left empty is not asked for", func(t *testing.T) {
		t.Parallel()

		_, _, err := c.locate(api.RouteEndExec, runPath("01926f3a"))

		assert.EqualError(t, err, "workload-microsandbox: POST /v1/runs/{id}/execs/{exec}/end was asked for with no exec")
	})

	t.Run("what is not a route is not asked for", func(t *testing.T) {
		t.Parallel()

		_, _, err := c.locate("/v1/runs", nil)

		assert.Error(t, err)
	})
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "short", truncate("short", 8))
	assert.Equal(t, "abc…", truncate("abcdef", 3))
	assert.Equal(t, "ab…", truncate("abé", 3), "half a character is dropped rather than sent")
}
