package placement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
)

const gib = 1 << 30

var now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// offering is a node that spoke a moment ago and offers budget, of which it has
// given allocated away.
func offering(name string, budget vm.Resources, allocated vm.Resources) node.Node {
	return node.Node{
		Name: name,
		Role: node.OrchestratorRole,
		Capacity: vm.Info{
			Engine:    "microsandbox",
			CPUs:      budget.CPUs,
			Memory:    budget.Memory,
			Disk:      budget.Disk,
			Allocated: allocated,
		},
		LastHeartbeatAt: now.Add(-time.Second),
	}
}

func placementOf(nodes []node.Node, vms []vm.VM, overcommit float64) *Placement {
	p := New(nodesMemory.NewRepository(nodes...), vmsMemory.NewRepository(vms...), overcommit)
	p.now = func() time.Time { return now }

	return p
}

func TestPlacement_Pick(t *testing.T) {
	t.Parallel()

	small := vm.Resources{CPUs: 1, Memory: 1 * gib, Disk: 10 * gib}
	budget := vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 100 * gib}

	for name, tt := range map[string]struct {
		nodes      []node.Node
		vms        []vm.VM
		overcommit float64
		resources  vm.Resources
		want       string
		wantErr    error
	}{
		"the node with the most memory left": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{Memory: 6 * gib}),
				offering("node-b", budget, vm.Resources{Memory: 2 * gib}),
			},
			resources: small,
			want:      "node-b",
		},
		"then the one with the most disk left": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{Disk: 50 * gib}),
				offering("node-b", budget, vm.Resources{Disk: 10 * gib}),
			},
			resources: small,
			want:      "node-b",
		},
		"then by name, so the answer does not change": {
			nodes: []node.Node{
				offering("node-b", budget, vm.Resources{}),
				offering("node-a", budget, vm.Resources{}),
			},
			resources: small,
			want:      "node-a",
		},
		"memory is never given twice": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{Memory: 7 * gib}),
			},
			resources: vm.Resources{CPUs: 1, Memory: 2 * gib, Disk: gib},
			wantErr:   vm.ErrNoCapacity,
		},
		"and neither is disk": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{Disk: 95 * gib}),
			},
			resources: vm.Resources{CPUs: 1, Memory: gib, Disk: 10 * gib},
			wantErr:   vm.ErrNoCapacity,
		},
		"vCPUs may be given more than once, by the overcommit": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{CPUs: 14}),
			},
			overcommit: 4,
			resources:  vm.Resources{CPUs: 2, Memory: gib, Disk: gib},
			want:       "node-a",
		},
		"but not past it": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{CPUs: 15}),
			},
			overcommit: 4,
			resources:  vm.Resources{CPUs: 2, Memory: gib, Disk: gib},
			wantErr:    vm.ErrNoCapacity,
		},
		"what was asked of a node since it last reported counts against it": {
			nodes: []node.Node{
				offering("node-a", budget, vm.Resources{}),
				offering("node-b", budget, vm.Resources{Memory: 3 * gib}),
			},
			vms: []vm.VM{
				// asked of node-a, not reported yet: its engine has not counted them.
				{UUID: "01", NodeName: "node-a", CurrentState: vm.Scheduled, Resources: vm.Resources{CPUs: 1, Memory: 4 * gib, Disk: gib}},
				// reported already, so its engine counted it.
				{UUID: "02", NodeName: "node-b", CurrentState: vm.Running, LastHeartbeatAt: now, Resources: vm.Resources{CPUs: 1, Memory: 3 * gib, Disk: gib}},
				// failed before it got anywhere: it holds nothing.
				{UUID: "03", NodeName: "node-b", CurrentState: vm.Failed, Resources: vm.Resources{CPUs: 1, Memory: 4 * gib, Disk: gib}},
			},
			resources: small,
			want:      "node-b",
		},
		"a node that has gone quiet is not given one": {
			nodes: []node.Node{
				func() node.Node {
					n := offering("node-a", budget, vm.Resources{})
					n.LastHeartbeatAt = now.Add(-time.Minute)

					return n
				}(),
			},
			resources: small,
			wantErr:   vm.ErrNoCapacity,
		},
		"nor one that never said what it offers": {
			nodes: []node.Node{
				{Name: "node-a", Role: node.OrchestratorRole, LastHeartbeatAt: now},
			},
			resources: small,
			wantErr:   vm.ErrNoCapacity,
		},
		"nor the control plane": {
			nodes: []node.Node{
				func() node.Node {
					n := offering("controlplane", budget, vm.Resources{})
					n.Role = node.ControlPlaneRole

					return n
				}(),
			},
			resources: small,
			wantErr:   vm.ErrNoCapacity,
		},
		"no nodes at all": {
			resources: small,
			wantErr:   vm.ErrNoCapacity,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			picked, err := placementOf(tt.nodes, tt.vms, tt.overcommit).Pick(context.Background(), tt.resources)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, picked.Name)
		})
	}

	t.Run("a store that cannot be read is an error, not a full workload", func(t *testing.T) {
		t.Parallel()

		nodes := nodesMemory.NewRepository()
		nodes.Fail = errors.New("the database is away")

		_, err := New(nodes, vmsMemory.NewRepository(), 1).Pick(context.Background(), small)

		assert.Error(t, err)
		assert.NotErrorIs(t, err, vm.ErrNoCapacity)
	})
}

func TestPlacement_Fits(t *testing.T) {
	t.Parallel()

	budget := vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 100 * gib}

	held := vm.VM{
		UUID:            "01",
		NodeName:        "node-a",
		CurrentState:    vm.Running,
		LastHeartbeatAt: now,
		Resources:       vm.Resources{CPUs: 2, Memory: 4 * gib, Disk: 20 * gib},
	}

	for name, tt := range map[string]struct {
		nodes     []node.Node
		vm        vm.VM
		resources vm.Resources
		want      bool
	}{
		"growing into the room the node has left": {
			// the node counts the 4 GiB the vm has, and 2 more of others.
			nodes:     []node.Node{offering("node-a", budget, vm.Resources{CPUs: 3, Memory: 6 * gib, Disk: 30 * gib})},
			vm:        held,
			resources: vm.Resources{CPUs: 2, Memory: 6 * gib, Disk: 20 * gib},
			want:      true,
		},
		"growing past it": {
			nodes:     []node.Node{offering("node-a", budget, vm.Resources{CPUs: 3, Memory: 6 * gib, Disk: 30 * gib})},
			vm:        held,
			resources: vm.Resources{CPUs: 2, Memory: 7 * gib, Disk: 20 * gib},
			want:      false,
		},
		"a node that has not said what it offers refuses for itself": {
			nodes:     []node.Node{{Name: "node-a", LastHeartbeatAt: now}},
			vm:        held,
			resources: vm.Resources{CPUs: 2, Memory: 64 * gib, Disk: 20 * gib},
			want:      true,
		},
		"a node nobody knows refuses for itself too": {
			vm:        held,
			resources: vm.Resources{CPUs: 2, Memory: 64 * gib, Disk: 20 * gib},
			want:      true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fits, err := placementOf(tt.nodes, []vm.VM{tt.vm}, 1).Fits(context.Background(), &tt.vm, tt.resources)

			require.NoError(t, err)
			assert.Equal(t, tt.want, fits)
		})
	}
}
