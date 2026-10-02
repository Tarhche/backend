package placement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
)

var now = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

// candidates is a scheduler that picks the first node it is offered, and
// remembers which ones those were: which nodes survive the filters is what
// these tests are about, not which of them the scheduler would prefer.
type candidates struct {
	offered []string
}

var _ task.Scheduler = &candidates{}

func (c *candidates) Pick(_ *task.Task, nodes []node.Node) node.Node {
	for _, n := range nodes {
		c.offered = append(c.offered, n.Name)
	}

	return nodes[0]
}

func speaking(name string, offers ...runtime.Offer) node.Node {
	return node.Node{Name: name, Runtimes: offers, LastHeartbeatAt: now.Add(-time.Second)}
}

func silent(name string, offers ...runtime.Offer) node.Node {
	return node.Node{Name: name, Runtimes: offers, LastHeartbeatAt: now.Add(-time.Minute)}
}

func containers() runtime.Offer {
	return runtime.Offer{
		Class:   runtime.Sysbox,
		Driver:  "container",
		Healthy: true,
		Capabilities: runtime.Capabilities{
			Isolation:       runtime.IsolationContainer,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    true,
			TTY:             true,
			RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
		},
	}
}

func microVMs() runtime.Offer {
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
			MaxMemory:       2 << 30,
			MaxCPU:          2,
		},
		Capacity: runtime.Capacity{CPU: 8, Memory: 8 << 30, Reserved: true},
	}
}

func unhealthy(offer runtime.Offer) runtime.Offer {
	offer.Healthy = false
	offer.Reason = "vmhost cannot be reached"

	return offer
}

func placementOver(t *testing.T, nodes ...node.Node) (*Placement, *candidates) {
	t.Helper()

	var repository nodesMock.MockNodesRepository
	repository.On("GetAll", mock.Anything, uint(0), uint(NodesLimit)).Return(nodes, nil).Once()
	t.Cleanup(func() { repository.AssertExpectations(t) })

	scheduler := &candidates{}
	p := New(&repository, scheduler)
	p.now = func() time.Time { return now }

	return p, scheduler
}

func firecrackerTask() *task.Task {
	return &task.Task{
		UUID:           "task-uuid",
		Runtime:        runtime.Firecracker,
		NetworkPolicy:  network.PolicyIsolated,
		ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 256 << 20, Disk: 1 << 30},
	}
}

func TestPlacement_Place(t *testing.T) {
	t.Parallel()

	t.Run("a task goes to a speaking node whose offer of its class is healthy", func(t *testing.T) {
		t.Parallel()

		p, scheduler := placementOver(t,
			speaking("workload-orchestrator-01", containers()),
			speaking("workload-orchestrator-02", containers(), microVMs()),
			speaking("workload-orchestrator-03", containers(), unhealthy(microVMs())),
			silent("workload-orchestrator-04", microVMs()),
		)

		selected, err := p.Place(context.Background(), firecrackerTask())
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-02", selected.Name)
		assert.Equal(t, []string{"workload-orchestrator-02"}, scheduler.offered, "the only node that can run it right now")
	})

	t.Run("a class no node offers at all is not something waiting changes", func(t *testing.T) {
		t.Parallel()

		p, _ := placementOver(t,
			speaking("workload-orchestrator-01", containers()),
			silent("workload-orchestrator-02", containers()),
		)

		_, err := p.Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, ErrNoNodeOffersRuntime)
	})

	t.Run("a class offered only unhealthy is waited for", func(t *testing.T) {
		t.Parallel()

		// vmhost is being redeployed: the class is coming back.
		p, _ := placementOver(t,
			speaking("workload-orchestrator-01", containers(), unhealthy(microVMs())),
		)

		_, err := p.Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, ErrNoNodeReady)
	})

	t.Run("a class offered only by a node that is not speaking is waited for", func(t *testing.T) {
		t.Parallel()

		// the node is being redeployed, and it is coming back.
		p, _ := placementOver(t,
			speaking("workload-orchestrator-01", containers()),
			silent("workload-orchestrator-02", microVMs()),
		)

		_, err := p.Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, ErrNoNodeReady)
	})

	t.Run("a platform none of whose nodes has spoken yet is waited for", func(t *testing.T) {
		t.Parallel()

		p, _ := placementOver(t)

		_, err := p.Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, ErrNoNodeReady)
	})

	t.Run("a node from before there were classes runs sysbox", func(t *testing.T) {
		t.Parallel()

		// an orchestrator that says nothing about classes, during a deploy
		// that has reached the control plane before it.
		legacy := speaking("workload-orchestrator-01")

		p, scheduler := placementOver(t, legacy)

		selected, err := p.Place(context.Background(), &task.Task{
			UUID:          "task-uuid",
			NetworkPolicy: network.PolicyPublic,
			StackUUID:     "stack-uuid",
			ReadOnly:      true,
			RestartPolicy: "unless-stopped",
		})
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-01", selected.Name, "a task naming no class is sysbox, which the node runs")
		assert.Equal(t, []string{"workload-orchestrator-01"}, scheduler.offered)
	})

	t.Run("and nothing else", func(t *testing.T) {
		t.Parallel()

		p, _ := placementOver(t, speaking("workload-orchestrator-01"))

		_, err := p.Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, ErrNoNodeOffersRuntime)
	})

	t.Run("a node whose class cannot give the task what it asks for is passed over", func(t *testing.T) {
		t.Parallel()

		tooSmall := microVMs()
		tooSmall.Capabilities.MaxMemory = 128 << 20

		p, scheduler := placementOver(t,
			speaking("workload-orchestrator-01", tooSmall),
			speaking("workload-orchestrator-02", microVMs()),
		)

		selected, err := p.Place(context.Background(), firecrackerTask())
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-02", selected.Name)
		assert.Equal(t, []string{"workload-orchestrator-02"}, scheduler.offered)
	})

	t.Run("and when none can, the task waits rather than fails", func(t *testing.T) {
		t.Parallel()

		withoutStacks := microVMs()
		withoutStacks.Capabilities.StackNetworks = false

		p, _ := placementOver(t, speaking("workload-orchestrator-01", withoutStacks))

		service := firecrackerTask()
		service.StackUUID = "stack-uuid"

		_, err := p.Place(context.Background(), service)

		assert.ErrorIs(t, err, ErrNoNodeReady)
	})

	t.Run("what the nodes cannot be read for is reported", func(t *testing.T) {
		t.Parallel()

		unreachable := errors.New("the database is unreachable")

		var repository nodesMock.MockNodesRepository
		repository.On("GetAll", mock.Anything, uint(0), uint(NodesLimit)).Return(nil, unreachable).Once()

		_, err := New(&repository, &candidates{}).Place(context.Background(), firecrackerTask())

		assert.ErrorIs(t, err, unreachable)
	})
}

func TestPlacement_PlaceTogether(t *testing.T) {
	t.Parallel()

	t.Run("services go together to a node that can run every one of them", func(t *testing.T) {
		t.Parallel()

		noReadOnly := microVMs()
		noReadOnly.Capabilities.ReadOnlyRoot = false

		p, scheduler := placementOver(t,
			speaking("workload-orchestrator-01", noReadOnly),
			speaking("workload-orchestrator-02", microVMs()),
		)

		selected, err := p.PlaceTogether(context.Background(), runtime.Firecracker,
			Need{NetworkPolicy: network.PolicyPublic, StackNetwork: true, Memory: 256 << 20, CPU: 1},
			Need{NetworkPolicy: network.PolicyIsolated, StackNetwork: true, ReadOnly: true, Memory: 512 << 20, CPU: 0.5},
		)
		require.NoError(t, err)

		assert.Equal(t, "workload-orchestrator-02", selected.Name)
		assert.Equal(t, []string{"workload-orchestrator-02"}, scheduler.offered)
	})

	t.Run("a stack of a class nobody offers is not placed either", func(t *testing.T) {
		t.Parallel()

		p, _ := placementOver(t, speaking("workload-orchestrator-01", containers()))

		_, err := p.PlaceTogether(context.Background(), runtime.Firecracker, Need{StackNetwork: true})

		assert.ErrorIs(t, err, ErrNoNodeOffersRuntime)
	})
}

func TestNeed_MetBy(t *testing.T) {
	t.Parallel()

	capabilities := microVMs().Capabilities

	testCases := []struct {
		name string
		need Need
		want bool
	}{
		{name: "what a class can give", need: Need{NetworkPolicy: network.PolicyPublic, RestartPolicy: "on-failure:3", Memory: 1 << 30, CPU: 2}, want: true},
		{name: "no policy named is the default one", need: Need{}, want: true},
		{name: "a network policy it cannot honour", need: Need{NetworkPolicy: "host"}, want: false},
		{name: "a read-only root it gives", need: Need{ReadOnly: true, StackNetwork: true}, want: true},
		{name: "a restart policy it does not apply", need: Need{RestartPolicy: "sometimes"}, want: false},
		{name: "more memory than one task may have", need: Need{Memory: 2<<30 + 1}, want: false},
		{name: "more CPUs than one task may have", need: Need{CPU: 2.5}, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, testCase.need.MetBy(capabilities))
		})
	}

	t.Run("a class that names no bound has none", func(t *testing.T) {
		t.Parallel()

		unbounded := containers().Capabilities

		assert.True(t, Need{Memory: 64 << 30, CPU: 32}.MetBy(unbounded))
	})

	t.Run("a read-only root or a stack's network a class does not give", func(t *testing.T) {
		t.Parallel()

		bare := runtime.Capabilities{NetworkPolicies: []network.Policy{network.PolicyIsolated}}

		assert.False(t, Need{ReadOnly: true}.MetBy(bare))
		assert.False(t, Need{StackNetwork: true}.MetBy(bare))
		assert.True(t, Need{}.MetBy(bare))
	})
}

func TestOffers(t *testing.T) {
	t.Parallel()

	t.Run("a node says what it offers", func(t *testing.T) {
		t.Parallel()

		n := speaking("workload-orchestrator-01", containers(), microVMs())

		offer, ok := OfferOf(n, runtime.Firecracker)
		require.True(t, ok)
		assert.Equal(t, runtime.Firecracker, offer.Class)

		_, ok = OfferOf(n, "gvisor")
		assert.False(t, ok)
	})

	t.Run("a node from before there were classes offers sysbox, with what a container can do", func(t *testing.T) {
		t.Parallel()

		offers := Offers(speaking("workload-orchestrator-01"))
		require.Len(t, offers, 1)

		assert.Equal(t, runtime.Sysbox, offers[0].Class)
		assert.True(t, offers[0].Healthy)
		assert.Equal(t, runtime.IsolationContainer, offers[0].Capabilities.Isolation)
		assert.ElementsMatch(t, []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic}, offers[0].Capabilities.NetworkPolicies)
		assert.True(t, offers[0].Capabilities.StackNetworks)
		assert.False(t, offers[0].Capabilities.DiskLimit, "docker does not hold a task to its disk")
	})

	t.Run("a node is healthy while it speaks", func(t *testing.T) {
		t.Parallel()

		assert.True(t, Healthy(speaking("workload-orchestrator-01"), now))
		assert.False(t, Healthy(silent("workload-orchestrator-01"), now))
		assert.False(t, Healthy(node.Node{Name: "never-spoke"}, now))
	})
}
