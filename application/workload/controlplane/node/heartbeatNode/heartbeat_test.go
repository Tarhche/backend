package heartbeatNode

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
)

func message(t *testing.T, heartbeat events.Heartbeat) []byte {
	t.Helper()

	payload, err := json.Marshal(heartbeat)
	require.NoError(t, err)

	return payload
}

func TestHeartbeat_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	t.Run("what a node says of itself is written down", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		capacity := vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 6, Memory: 7680 << 20, Disk: 100 << 30, Allocated: vm.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30}}

		require.NoError(t, NewHeartbeatHandler(nodes).Handle(ctx, message(t, events.Heartbeat{
			Name:     "node-1",
			Role:     node.OrchestratorRole,
			Stats:    node.Stats{PIDs: 7},
			Capacity: capacity,
			At:       at,
		})))

		n, err := nodes.GetOne(ctx, "node-1")
		require.NoError(t, err)
		assert.Equal(t, node.OrchestratorRole, n.Role)
		assert.Equal(t, node.Stats{PIDs: 7}, n.Stats)
		assert.Equal(t, capacity, n.Capacity, "what it offers to VMs, which they are placed by")
		assert.Equal(t, at, n.LastHeartbeatAt, "when it was last heard, which says whether it is alive")
	})

	t.Run("one from a node that still says what its kinds hold in it is heard for what it says of the node", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()

		require.NoError(t, NewHeartbeatHandler(nodes).Handle(ctx, []byte(`{"Name":"node-1","Role":"orchestrator","At":"2026-10-06T12:00:00Z","observations":{"vm":{"instances":[]}}}`)))

		n, err := nodes.GetOne(ctx, "node-1")
		require.NoError(t, err)
		assert.Equal(t, at, n.LastHeartbeatAt)
	})

	t.Run("a node that cannot be written down is heard again", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		nodes.Fail = errors.New("the database is gone")

		err := NewHeartbeatHandler(nodes).Handle(ctx, message(t, events.Heartbeat{Name: "node-1", At: at}))

		assert.ErrorIs(t, err, nodes.Fail)
	})
}
