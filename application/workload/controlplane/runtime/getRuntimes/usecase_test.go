package getRuntimes

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
)

var now = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func speaking(name string, offers ...runtime.Offer) node.Node {
	return node.Node{Name: name, Runtimes: offers, LastHeartbeatAt: now.Add(-time.Second)}
}

func silent(name string, offers ...runtime.Offer) node.Node {
	return node.Node{Name: name, Runtimes: offers, LastHeartbeatAt: now.Add(-time.Hour)}
}

func microVMs(memory uint64) runtime.Offer {
	return runtime.Offer{
		Class:   runtime.Firecracker,
		Driver:  "microvm",
		Healthy: true,
		Capabilities: runtime.Capabilities{
			Isolation:       runtime.IsolationMicroVM,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    true,
			DiskLimit:       true,
			TTY:             true,
			RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
			MinMemory:       128 << 20,
			MaxMemory:       4 << 30,
			MaxCPU:          4,
			Architectures:   []string{"amd64"},
		},
		Capacity: runtime.Capacity{CPU: 8, AllocatedCPU: 2, Memory: memory, AllocatedMemory: 1 << 30, Reserved: true},
	}
}

func useCase(t *testing.T, classes allowed.Classes, nodes ...node.Node) *UseCase {
	t.Helper()

	var repository nodesMock.MockNodesRepository
	repository.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return(nodes, nil).Once()
	t.Cleanup(func() { repository.AssertExpectations(t) })

	uc := NewUseCase(&repository, classes)
	uc.now = func() time.Time { return now }

	return uc
}

func bothClasses(t *testing.T) allowed.Classes {
	t.Helper()

	classes, err := allowed.New([]runtime.Class{runtime.Sysbox, runtime.Firecracker}, runtime.Sysbox)
	require.NoError(t, err)

	return classes
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("every allowed class, in the order the platform names them", func(t *testing.T) {
		t.Parallel()

		classes, err := allowed.New([]runtime.Class{runtime.Firecracker, runtime.Sysbox}, runtime.Sysbox)
		require.NoError(t, err)

		response, err := useCase(t, classes, speaking("workload-orchestrator-01")).Execute(context.Background())
		require.NoError(t, err)
		require.Len(t, response.Items, 2)

		assert.Equal(t, runtime.Firecracker, response.Items[0].Class)
		assert.False(t, response.Items[0].Default)
		assert.Equal(t, runtime.Sysbox, response.Items[1].Class)
		assert.True(t, response.Items[1].Default)
	})

	t.Run("a class is what the nodes that can run it right now can all do, and what they hold between them", func(t *testing.T) {
		t.Parallel()

		smaller := microVMs(16 << 30)
		smaller.Capabilities.MaxMemory = 2 << 30
		smaller.Capabilities.MinMemory = 256 << 20
		smaller.Capabilities.MaxCPU = 0
		smaller.Capabilities.ReadOnlyRoot = false
		smaller.Capabilities.RestartPolicies = []string{"no", "always"}
		smaller.Capabilities.Architectures = []string{"arm64", "amd64"}

		down := microVMs(64 << 30)
		down.Healthy = false
		down.Reason = "vmhost cannot be reached"

		response, err := useCase(t, bothClasses(t),
			speaking("workload-orchestrator-01", microVMs(8<<30)),
			speaking("workload-orchestrator-02", smaller),
			speaking("workload-orchestrator-03", down),
			silent("workload-orchestrator-04", microVMs(32<<30)),
		).Execute(context.Background())
		require.NoError(t, err)

		firecracker := response.Items[1]
		assert.Equal(t, runtime.Firecracker, firecracker.Class)
		assert.True(t, firecracker.Available)
		assert.Equal(t, 2, firecracker.Nodes, "a node whose vmhost is down, and one that is not speaking, are not counted")

		assert.Equal(t, runtime.Capabilities{
			Isolation:       runtime.IsolationMicroVM,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    false,
			DiskLimit:       true,
			TTY:             true,
			RestartPolicies: []string{"no", "always"},
			MinMemory:       256 << 20,
			MaxMemory:       2 << 30,
			MaxCPU:          4,
			Architectures:   []string{"amd64"},
		}, firecracker.Capabilities)

		assert.Equal(t, runtime.Capacity{
			CPU:             16,
			AllocatedCPU:    4,
			Memory:          24 << 30,
			AllocatedMemory: 2 << 30,
			Reserved:        true,
		}, firecracker.Capacity)
	})

	t.Run("a node from before there were classes runs sysbox, as a container", func(t *testing.T) {
		t.Parallel()

		response, err := useCase(t, bothClasses(t),
			speaking("workload-orchestrator-01"),
			speaking("workload-orchestrator-02"),
		).Execute(context.Background())
		require.NoError(t, err)

		sysbox := response.Items[0]
		assert.True(t, sysbox.Default)
		assert.True(t, sysbox.Available)
		assert.Equal(t, 2, sysbox.Nodes)
		assert.Equal(t, runtime.IsolationContainer, sysbox.Capabilities.Isolation)
		assert.True(t, sysbox.Capabilities.StackNetworks)
		assert.False(t, sysbox.Capabilities.DiskLimit)

		assert.False(t, response.Items[1].Available, "it offers nothing else")
	})

	t.Run("an allowed class nothing can run is listed, unavailable, with every list empty", func(t *testing.T) {
		t.Parallel()

		response, err := useCase(t, bothClasses(t), speaking("workload-orchestrator-01")).Execute(context.Background())
		require.NoError(t, err)

		encoded, err := json.Marshal(response.Items[1])
		require.NoError(t, err)

		assert.JSONEq(t, `{"class":"firecracker","default":false,"available":false,"nodes":0,
			"capabilities":{"isolation":"","network_policies":[],"stack_networks":false,"read_only_root":false,"disk_limit":false,"tty":false,"restart_policies":[],"min_memory":0,"max_memory":0,"max_cpu":0,"architectures":[]},
			"capacity":{"cpu":0,"allocated_cpu":0,"memory":0,"allocated_memory":0,"reserved":false}}`, string(encoded))
	})

	t.Run("nodes that disagree on how a class keeps tasks apart say nothing about it", func(t *testing.T) {
		t.Parallel()

		odd := microVMs(8 << 30)
		odd.Capabilities.Isolation = "something-else"

		response, err := useCase(t, bothClasses(t),
			speaking("workload-orchestrator-01", microVMs(8<<30)),
			speaking("workload-orchestrator-02", odd),
		).Execute(context.Background())
		require.NoError(t, err)

		assert.Empty(t, response.Items[1].Capabilities.Isolation)
	})

	t.Run("what the nodes cannot be read for is reported", func(t *testing.T) {
		t.Parallel()

		unreachable := errors.New("the database is unreachable")

		var repository nodesMock.MockNodesRepository
		repository.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return(nil, unreachable).Once()

		_, err := NewUseCase(&repository, bothClasses(t)).Execute(context.Background())

		assert.ErrorIs(t, err, unreachable)
	})

	t.Run("what a node said is never written into", func(t *testing.T) {
		t.Parallel()

		offer := microVMs(8 << 30)
		other := microVMs(8 << 30)
		other.Capabilities.NetworkPolicies = []network.Policy{network.PolicyIsolated}

		first := speaking("workload-orchestrator-01", offer)

		_, err := useCase(t, bothClasses(t), first, speaking("workload-orchestrator-02", other)).Execute(context.Background())
		require.NoError(t, err)

		assert.Equal(t, []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic}, first.Runtimes[0].Capabilities.NetworkPolicies)
	})
}
