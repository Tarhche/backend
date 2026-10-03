package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestClient_Runtimes(t *testing.T) {
	t.Parallel()

	t.Run("the classes are what the control plane says they are", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/runtimes", r.URL.Path)

			_, _ = rw.Write([]byte(`{"items":[
				{"class":"sysbox","default":true,"available":true,"nodes":3,
				 "capabilities":{"isolation":"container","network_policies":["none","isolated","public"],"stack_networks":true,"read_only_root":true,"disk_limit":false,"tty":true,"restart_policies":["no","always","on-failure","unless-stopped"],"min_memory":0,"max_memory":0,"max_cpu":0,"architectures":["amd64"]},
				 "capacity":{"cpu":8,"allocated_cpu":1.5,"memory":12884901888,"allocated_memory":536870912,"reserved":false}},
				{"class":"firecracker","default":false,"available":false,"nodes":0,
				 "capabilities":{"isolation":"","network_policies":[],"stack_networks":false,"read_only_root":false,"disk_limit":false,"tty":false,"restart_policies":[],"min_memory":0,"max_memory":0,"max_cpu":0,"architectures":[]},
				 "capacity":{"cpu":0,"allocated_cpu":0,"memory":0,"allocated_memory":0,"reserved":false}}
			]}`))
		}))
		t.Cleanup(server.Close)

		c, err := New(server.URL)
		require.NoError(t, err)

		got, err := c.Runtimes(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, runtime.Availability{
			Class:     runtime.Sysbox,
			Default:   true,
			Available: true,
			Nodes:     3,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationContainer,
				NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				ReadOnlyRoot:    true,
				TTY:             true,
				RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
				Architectures:   []string{"amd64"},
			},
			Capacity: runtime.Capacity{CPU: 8, AllocatedCPU: 1.5, Memory: 12884901888, AllocatedMemory: 536870912},
		}, got[0])

		assert.Equal(t, runtime.Firecracker, got[1].Class)
		assert.False(t, got[1].Available)
	})

	t.Run("no classes at all is an empty list", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			_, _ = rw.Write([]byte(`{"items":null}`))
		}))
		t.Cleanup(server.Close)

		c, err := New(server.URL)
		require.NoError(t, err)

		got, err := c.Runtimes(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestTaskPayload_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task the control plane names a class for has it", func(t *testing.T) {
		t.Parallel()

		var payload taskPayload
		require.NoError(t, json.Unmarshal([]byte(`{"uuid":"t-1","runtime":"firecracker","node_name":"workload-orchestrator-01"}`), &payload))

		got := payload.toTask()

		assert.Equal(t, runtime.Firecracker, got.Runtime)
		assert.Equal(t, "workload-orchestrator-01", got.NodeName)
	})

	t.Run("a task from before there were classes ran under sysbox", func(t *testing.T) {
		t.Parallel()

		var payload taskPayload
		require.NoError(t, json.Unmarshal([]byte(`{"uuid":"t-1"}`), &payload))

		assert.Equal(t, runtime.Sysbox, payload.toTask().Runtime)
	})

	t.Run("a stack's class is read the same way", func(t *testing.T) {
		t.Parallel()

		var payload stackPayload
		require.NoError(t, json.Unmarshal([]byte(`{"uuid":"s-1","runtime":"firecracker"}`), &payload))

		assert.Equal(t, runtime.Firecracker, payload.toStack().Runtime)
	})
}
