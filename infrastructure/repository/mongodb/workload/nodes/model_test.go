package nodes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

func TestNodeBson_Runtimes(t *testing.T) {
	t.Parallel()

	t.Run("what a node offers is stored, and read back as it was said", func(t *testing.T) {
		t.Parallel()

		offered := node.Node{
			Name:  "workload-orchestrator-01",
			Role:  node.OrchestratorRole,
			Stats: node.Stats{CPUPercent: 12.5, MemoryUsage: 1 << 30},
			Runtimes: []runtime.Offer{
				{
					Class:   runtime.Sysbox,
					Driver:  "container",
					Version: "29.0.1",
					Healthy: true,
					Capabilities: runtime.Capabilities{
						Isolation:       runtime.IsolationContainer,
						NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
						StackNetworks:   true,
						ReadOnlyRoot:    true,
						TTY:             true,
						RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
						MinMemory:       6 << 20,
						Architectures:   []string{"amd64"},
					},
					Capacity: runtime.Capacity{CPU: 8, AllocatedCPU: 1.5, Memory: 12 << 30, AllocatedMemory: 512 << 20},
				},
				{
					Class:   runtime.Firecracker,
					Driver:  "microvm",
					Healthy: false,
					Reason:  "vmhost cannot be reached",
					Capabilities: runtime.Capabilities{
						Isolation:       runtime.IsolationMicroVM,
						NetworkPolicies: []network.Policy{network.PolicyIsolated},
						DiskLimit:       true,
						MaxMemory:       4 << 30,
						MaxCPU:          2,
					},
					Capacity: runtime.Capacity{CPU: 8, Memory: 16 << 30, Disk: 100 << 30, AllocatedDisk: 1 << 30, Reserved: true},
				},
			},
			LastHeartbeatAt: time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
		}

		encoded, err := bson.Marshal(toBson(&offered))
		require.NoError(t, err)

		var stored NodeBson
		require.NoError(t, bson.Unmarshal(encoded, &stored))

		assert.Equal(t, offered, toNode(&stored))
	})

	t.Run("a node that says nothing about classes is stored as offering none, over what it said before", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(toBson(&node.Node{Name: "workload-orchestrator-01"}))
		require.NoError(t, err)

		// written out, rather than left out: a heartbeat says everything the
		// node offers, and one that offers nothing has stopped offering what
		// it offered before.
		var raw bson.M
		require.NoError(t, bson.Unmarshal(encoded, &raw))
		assert.Contains(t, raw, "runtimes")
		assert.Nil(t, raw["runtimes"])

		var stored NodeBson
		require.NoError(t, bson.Unmarshal(encoded, &stored))
		assert.Empty(t, toNode(&stored).Runtimes)
	})

	t.Run("a node stored before there were classes offers none it has said", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(bson.M{"_id": "workload-orchestrator-01", "name": "workload-orchestrator-01", "role": "orchestrator"})
		require.NoError(t, err)

		var stored NodeBson
		require.NoError(t, bson.Unmarshal(encoded, &stored))

		assert.Nil(t, toNode(&stored).Runtimes)
	})
}
