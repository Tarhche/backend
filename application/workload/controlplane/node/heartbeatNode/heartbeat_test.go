package heartbeatNode

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// heard is one heartbeat an observer was handed.
type heard struct {
	node    string
	at      time.Time
	reports map[string]kind.Report[json.RawMessage]
}

// recording is an observer that keeps what it is handed.
type recording struct {
	lock  sync.Mutex
	heard []heard
}

func (r *recording) Heartbeat(_ context.Context, nodeName string, at time.Time, reports map[string]kind.Report[json.RawMessage]) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.heard = append(r.heard, heard{node: nodeName, at: at, reports: reports})
}

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

	reports := map[string]kind.Report[json.RawMessage]{
		"fan": {Instances: []kind.Observation{{Kind: "fan", UUID: "fan-uuid", Status: json.RawMessage(`{"state":"running"}`)}}},
	}

	t.Run("what a node says of itself is written down, and what it observed is handed on", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		observer := &recording{}
		capacity := vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 6, Memory: 7680 << 20, Disk: 100 << 30, Allocated: vm.Resources{CPUs: 2, Memory: 1 << 30, Disk: 10 << 30}}

		require.NoError(t, NewHeartbeatHandler(nodes, observer).Handle(ctx, message(t, events.Heartbeat{
			Name:         "node-1",
			Role:         node.OrchestratorRole,
			Stats:        node.Stats{PIDs: 7},
			Capacity:     capacity,
			At:           at,
			Observations: reports,
		})))

		n, err := nodes.GetOne(ctx, "node-1")
		require.NoError(t, err)
		assert.Equal(t, node.OrchestratorRole, n.Role)
		assert.Equal(t, node.Stats{PIDs: 7}, n.Stats)
		assert.Equal(t, capacity, n.Capacity, "what it offers to VMs, which they are placed by")
		assert.Equal(t, at, n.LastHeartbeatAt)

		require.Len(t, observer.heard, 1)
		assert.Equal(t, "node-1", observer.heard[0].node)
		assert.Equal(t, at, observer.heard[0].at)
		assert.Equal(t, reports, observer.heard[0].reports)
	})

	t.Run("a node that runs no kinds says nothing of any", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, NewHeartbeatHandler(nodesMemory.NewRepository(), observer).Handle(ctx, message(t, events.Heartbeat{Name: "node-1", At: at})))

		assert.Empty(t, observer.heard)
	})

	t.Run("one that says not when is heard now", func(t *testing.T) {
		t.Parallel()

		observer := &recording{}

		require.NoError(t, NewHeartbeatHandler(nodesMemory.NewRepository(), observer).Handle(ctx, message(t, events.Heartbeat{Name: "node-1", Observations: reports})))

		require.Len(t, observer.heard, 1)
		assert.WithinDuration(t, time.Now(), observer.heard[0].at, time.Minute)
	})

	t.Run("with no observer, nothing is heard of what it holds", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()

		require.NoError(t, NewHeartbeatHandler(nodes, nil).Handle(ctx, message(t, events.Heartbeat{Name: "node-1", At: at, Observations: reports})))

		_, err := nodes.GetOne(ctx, "node-1")
		assert.NoError(t, err)
	})

	t.Run("a node that cannot be written down is heard again, all of it", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		nodes.Fail = errors.New("the database is gone")
		observer := &recording{}

		err := NewHeartbeatHandler(nodes, observer).Handle(ctx, message(t, events.Heartbeat{Name: "node-1", At: at, Observations: reports}))

		assert.ErrorIs(t, err, nodes.Fail)
		assert.Empty(t, observer.heard)
	})

	t.Run("what it observed is taken onto the resources it holds", func(t *testing.T) {
		t.Parallel()

		resources := resourcesMemory.NewRepository()
		_, err := resources.Create(ctx, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))
		require.NoError(t, err)

		observer := observe.NewObserver(kindstest.Registry(&kindstest.Fans{}), resources, slog.New(slog.DiscardHandler))

		require.NoError(t, NewHeartbeatHandler(nodesMemory.NewRepository(), observer).Handle(ctx, message(t, events.Heartbeat{
			Name:         kindstest.NodeName,
			At:           at,
			Observations: reports,
		})))

		stored, err := resources.GetOne(ctx, kindstest.Kind, "fan-uuid")
		require.NoError(t, err)
		assert.Equal(t, kindstest.Running, kindstest.Typed(stored).Status.State)
	})
}
