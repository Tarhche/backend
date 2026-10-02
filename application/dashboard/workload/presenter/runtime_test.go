package presenter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// TestNewRuntimes pins the dashboard's contract for the runtime classes, which
// the frontend's task form is built against: it changes only together with it.
func TestNewRuntimes(t *testing.T) {
	t.Parallel()

	t.Run("a class some node offers", func(t *testing.T) {
		t.Parallel()

		items := NewRuntimes([]runtime.Availability{{
			Class:     runtime.Sysbox,
			Default:   true,
			Available: true,
			Nodes:     3,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationContainer,
				NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				ReadOnlyRoot:    true,
				DiskLimit:       false,
				TTY:             true,
				RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
				Architectures:   []string{"amd64"},
			},
			Capacity: runtime.Capacity{
				CPU:             8,
				AllocatedCPU:    1.5,
				Memory:          12884901888,
				AllocatedMemory: 536870912,
				Disk:            100 << 30,
			},
		}})

		encoded, err := json.Marshal(map[string]any{"items": items})
		require.NoError(t, err)

		assert.JSONEq(t, `{"items":[{"class":"sysbox","default":true,"available":true,"nodes":3,
			"capabilities":{"isolation":"container","network_policies":["none","isolated","public"],"stack_networks":true,"read_only_root":true,"disk_limit":false,"tty":true,"restart_policies":["no","always","on-failure","unless-stopped"],"min_memory":0,"max_memory":0,"max_cpu":0,"architectures":["amd64"]},
			"capacity":{"cpu":8,"allocated_cpu":1.5,"memory":12884901888,"allocated_memory":536870912,"reserved":false}}]}`, string(encoded))
	})

	t.Run("a class nothing offers is listed, with every list empty rather than absent", func(t *testing.T) {
		t.Parallel()

		encoded, err := json.Marshal(NewRuntime(runtime.Availability{Class: runtime.Firecracker}))
		require.NoError(t, err)

		assert.JSONEq(t, `{"class":"firecracker","default":false,"available":false,"nodes":0,
			"capabilities":{"isolation":"","network_policies":[],"stack_networks":false,"read_only_root":false,"disk_limit":false,"tty":false,"restart_policies":[],"min_memory":0,"max_memory":0,"max_cpu":0,"architectures":[]},
			"capacity":{"cpu":0,"allocated_cpu":0,"memory":0,"allocated_memory":0,"reserved":false}}`, string(encoded))
	})
}

func TestNewTask_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task says which class runs it, and which node holds it", func(t *testing.T) {
		t.Parallel()

		presented := NewTask(task.Task{
			UUID:     "t-1",
			Runtime:  runtime.Firecracker,
			NodeName: "workload-orchestrator-01",
		}, ingressDomain, Owners{})

		encoded, err := json.Marshal(presented)
		require.NoError(t, err)

		var fields map[string]any
		require.NoError(t, json.Unmarshal(encoded, &fields))

		assert.Equal(t, "firecracker", fields["runtime"])
		assert.Equal(t, "workload-orchestrator-01", fields["node"])
	})

	t.Run("a task from before there were classes ran under sysbox, and one not placed yet is on no node", func(t *testing.T) {
		t.Parallel()

		presented := NewTask(task.Task{UUID: "t-1"}, ingressDomain, Owners{})

		assert.Equal(t, "sysbox", presented.Runtime)
		assert.Empty(t, presented.Node)
	})
}

func TestNewStack_Runtime(t *testing.T) {
	t.Parallel()

	presented := NewStack(workloadControlPlane.Stack{
		Stack: stack.Stack{UUID: "s-1", Runtime: runtime.Firecracker, CreatedAt: time.Now()},
	}, ingressDomain, Owners{})

	encoded, err := json.Marshal(presented)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))

	assert.Equal(t, "firecracker", fields["runtime"])
}
