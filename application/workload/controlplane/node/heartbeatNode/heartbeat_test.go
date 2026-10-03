package heartbeatNode

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
)

func beat(t *testing.T, heartbeat events.Heartbeat) []byte {
	t.Helper()

	payload, err := json.Marshal(heartbeat)
	require.NoError(t, err)

	return payload
}

func TestHeartbeat_Handle(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	offers := []runtime.Offer{
		{Class: runtime.Sysbox, Driver: "container", Healthy: true},
		{Class: runtime.Firecracker, Driver: "microvm", Healthy: false, Reason: "vmhost cannot be reached"},
	}

	t.Run("what a node offers is stored as it said it", func(t *testing.T) {
		t.Parallel()

		var nodes nodesMock.MockNodesRepository

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").
			Return(node.Node{Name: "workload-orchestrator-01", Runtimes: offers[:1]}, nil).Once()

		var saved node.Node
		nodes.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { saved = *args.Get(1).(*node.Node) }).
			Return("workload-orchestrator-01", nil).Once()
		defer nodes.AssertExpectations(t)

		require.NoError(t, NewHeartbeatHandler(&nodes).Handle(context.Background(), beat(t, events.Heartbeat{
			Name:     "workload-orchestrator-01",
			Role:     node.OrchestratorRole,
			Stats:    node.Stats{CPUPercent: 12.5},
			Runtimes: offers,
			At:       at,
		})))

		assert.Equal(t, offers, saved.Runtimes)
		assert.Equal(t, at, saved.LastHeartbeatAt.UTC())
		assert.Equal(t, 12.5, saved.Stats.CPUPercent)
	})

	t.Run("a node that says nothing about classes offers none it has said, whatever it said before", func(t *testing.T) {
		t.Parallel()

		var nodes nodesMock.MockNodesRepository

		// an orchestrator rolled back to one from before there were
		// classes: what it offered before is no longer true.
		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").
			Return(node.Node{Name: "workload-orchestrator-01", Runtimes: offers}, nil).Once()

		var saved node.Node
		nodes.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { saved = *args.Get(1).(*node.Node) }).
			Return("workload-orchestrator-01", nil).Once()

		require.NoError(t, NewHeartbeatHandler(&nodes).Handle(context.Background(), []byte(`{"Name":"workload-orchestrator-01","Role":"orchestrator","Stats":{},"At":"2026-10-02T12:00:00Z"}`)))

		assert.Empty(t, saved.Runtimes)
	})

	t.Run("a node heard of for the first time is stored with what it offers", func(t *testing.T) {
		t.Parallel()

		var nodes nodesMock.MockNodesRepository

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-04").Return(node.Node{}, domain.ErrNotExists).Once()

		var saved node.Node
		nodes.On("Save", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { saved = *args.Get(1).(*node.Node) }).
			Return("workload-orchestrator-04", nil).Once()

		require.NoError(t, NewHeartbeatHandler(&nodes).Handle(context.Background(), beat(t, events.Heartbeat{
			Name:     "workload-orchestrator-04",
			Role:     node.OrchestratorRole,
			Runtimes: offers,
			At:       at,
		})))

		assert.Equal(t, "workload-orchestrator-04", saved.Name)
		assert.Equal(t, offers, saved.Runtimes)
	})
}
