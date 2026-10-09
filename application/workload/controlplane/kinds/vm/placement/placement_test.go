package placement

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

const gib = 1 << 30

// now is when every test here asks.
var now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// spoke is a node that last spoke ago, offering cpus vCPUs, memory and disk
// in GiB, with allocated given away already as its engine counts it.
func spoke(name string, ago time.Duration, cpus uint, memory uint64, disk uint64, allocated vm.Resources) node.Node {
	return node.Node{
		Name:            name,
		Role:            node.OrchestratorRole,
		Capacity:        vm.Info{Engine: "microsandbox", CPUs: cpus, Memory: memory * gib, Disk: disk * gib, Allocated: allocated},
		LastHeartbeatAt: now.Add(-ago),
	}
}

// on is a VM given resources on a node, in state since it was last observed
// at observed: zero for one its node has not reported yet.
func on(uuid string, nodeName string, resources vmKind.Resources, observed time.Time, state kind.State) vmKind.VM {
	return vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Slug: "vm-" + uuid, OwnerUUID: "owner", Node: nodeName},
		Spec:     vmKind.Spec{Flavor: vmKind.FlavorMachine, Resources: resources},
		Status:   vmKind.Status{Status: kind.Status{State: state, Expected: vmKind.Running, Since: observed, ObservedAt: observed}},
	}
}

// since is v in its state since a moment before it was last observed.
func since(v vmKind.VM, at time.Time) vmKind.VM {
	v.Status.Since = at

	return v
}

func placementOf(t *testing.T, nodes []node.Node, vms ...vmKind.VM) *Placement {
	t.Helper()

	held := make([]resource.Record, len(vms))
	for i := range vms {
		raw, err := kind.Encode(vms[i])
		require.NoError(t, err)

		held[i] = resource.Record{Raw: raw}
	}

	p := New(nodesMemory.NewRepository(nodes...), records.New(resourcesMemory.NewRepository(held...)), 2)
	p.now = func() time.Time { return now }

	return p
}

func TestPlacement_Pick(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	asked := vmKind.Resources{CPUs: 2, Memory: 4 * gib, Disk: 20 * gib}

	for name, tt := range map[string]struct {
		nodes []node.Node
		vms   []vmKind.VM
		want  string
	}{
		"the node with the most memory left": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{Memory: 40 * gib}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 10 * gib}),
			},
			want: "b",
		},
		"then the most disk left": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{Disk: 500 * gib}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Disk: 100 * gib}),
			},
			want: "b",
		},
		"then by name": {
			nodes: []node.Node{spoke("b", time.Second, 16, 64, 1000, vm.Resources{}), spoke("a", time.Second, 16, 64, 1000, vm.Resources{})},
			want:  "a",
		},
		"one that has not spoken lately is passed over": {
			nodes: []node.Node{
				spoke("a", time.Minute, 16, 64, 1000, vm.Resources{}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 50 * gib}),
			},
			want: "b",
		},
		"and so is one that has not said what it offers, and the control plane": {
			nodes: []node.Node{
				{Name: "a", Role: node.OrchestratorRole, LastHeartbeatAt: now},
				func() node.Node {
					n := spoke("control", time.Second, 64, 640, 10000, vm.Resources{})
					n.Role = node.ControlPlaneRole
					return n
				}(),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 50 * gib}),
			},
			want: "b",
		},
		"vcpus are given twice over": {
			nodes: []node.Node{spoke("a", time.Second, 4, 64, 1000, vm.Resources{CPUs: 6})},
			want:  "a",
		},
		"what was asked of a node since it last spoke is counted as given": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 10 * gib}),
			},
			vms: []vmKind.VM{
				on("new", "a", vmKind.Resources{CPUs: 1, Memory: 20 * gib, Disk: 10 * gib}, time.Time{}, vmKind.Scheduled),
				on("answered", "a", vmKind.Resources{CPUs: 1, Memory: 20 * gib, Disk: 10 * gib}, now, vmKind.Running),
			},
			want: "b",
		},
		"while what its engine counts already is not counted twice, to the millisecond its heartbeat is kept to": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{Memory: 20 * gib}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 30 * gib}),
			},
			vms: []vmKind.VM{
				on("reported", "a", vmKind.Resources{CPUs: 1, Memory: 20 * gib, Disk: 10 * gib}, now.Add(-time.Second).Add(999*time.Microsecond), vmKind.Running),
			},
			want: "a",
		},
		"nor is one that has run since before its node spoke, heard of again before its node is": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{Memory: 20 * gib}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 30 * gib}),
			},
			vms: []vmKind.VM{
				since(on("heard-first", "a", vmKind.Resources{CPUs: 1, Memory: 20 * gib, Disk: 10 * gib}, now, vmKind.Running), now.Add(-time.Hour)),
			},
			want: "a",
		},
		"while one at rest otherwise is counted as given once observed since, as a stopped one is once its node says what it gave it": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{Memory: 20 * gib}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 30 * gib}),
			},
			vms: []vmKind.VM{
				since(on("reconfigured", "a", vmKind.Resources{CPUs: 1, Memory: 20 * gib, Disk: 10 * gib}, now, vmKind.Stopped), now.Add(-time.Hour)),
			},
			want: "b",
		},
		"nor is one that failed or is on its way out": {
			nodes: []node.Node{
				spoke("a", time.Second, 16, 64, 1000, vm.Resources{}),
				spoke("b", time.Second, 16, 64, 1000, vm.Resources{Memory: 10 * gib}),
			},
			vms: []vmKind.VM{
				on("failed", "a", vmKind.Resources{CPUs: 1, Memory: 30 * gib, Disk: 10 * gib}, time.Time{}, vmKind.Failed),
				on("going", "a", vmKind.Resources{CPUs: 1, Memory: 30 * gib, Disk: 10 * gib}, time.Time{}, vmKind.Deleting),
			},
			want: "a",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			chosen, err := placementOf(t, tt.nodes, tt.vms...).Pick(ctx, asked)
			require.NoError(t, err)
			assert.Equal(t, tt.want, chosen.Name)
		})
	}

	for name, tt := range map[string]struct {
		nodes []node.Node
		vms   []vmKind.VM
	}{
		"no node at all": {},
		"none with the memory": {
			nodes: []node.Node{spoke("a", time.Second, 16, 64, 1000, vm.Resources{Memory: 61 * gib})},
		},
		"none with the disk": {
			nodes: []node.Node{spoke("a", time.Second, 16, 64, 1000, vm.Resources{Disk: 990 * gib})},
		},
		"none with the vcpus, given twice over": {
			nodes: []node.Node{spoke("a", time.Second, 4, 64, 1000, vm.Resources{CPUs: 7})},
		},
		"none with room once what was asked of it since is counted": {
			nodes: []node.Node{spoke("a", time.Second, 16, 64, 1000, vm.Resources{})},
			vms:   []vmKind.VM{on("new", "a", vmKind.Resources{CPUs: 1, Memory: 61 * gib, Disk: 10 * gib}, time.Time{}, vmKind.Scheduled)},
		},
	} {
		t.Run("no capacity: "+name, func(t *testing.T) {
			t.Parallel()

			_, err := placementOf(t, tt.nodes, tt.vms...).Pick(ctx, asked)
			assert.ErrorIs(t, err, vm.ErrNoCapacity)
		})
	}
}

func TestPlacement_Fits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// a node with 64 GiB, 20 of which its engine counts as given: 16 to the
	// VM that grows, reported, and 4 to another.
	nodes := []node.Node{spoke("a", time.Second, 16, 64, 1000, vm.Resources{CPUs: 4, Memory: 20 * gib, Disk: 200 * gib})}
	grows := on("grows", "a", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, now.Add(-time.Second), vmKind.Running)

	for name, tt := range map[string]struct {
		nodes     []node.Node
		vms       []vmKind.VM
		vm        vmKind.VM
		resources vmKind.Resources
		fits      bool
	}{
		"grown into what is left, what it has counted once": {
			nodes:     nodes,
			vms:       []vmKind.VM{grows},
			vm:        grows,
			resources: vmKind.Resources{CPUs: 4, Memory: 60 * gib, Disk: 300 * gib},
			fits:      true,
		},
		"grown past it": {
			nodes:     nodes,
			vms:       []vmKind.VM{grows},
			vm:        grows,
			resources: vmKind.Resources{CPUs: 4, Memory: 60*gib + 1, Disk: 300 * gib},
		},
		"one its node has not reported yet has nothing of it counted": {
			nodes:     nodes,
			vms:       []vmKind.VM{on("new", "a", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, time.Time{}, vmKind.Scheduled)},
			vm:        on("new", "a", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, time.Time{}, vmKind.Scheduled),
			resources: vmKind.Resources{CPUs: 4, Memory: 44 * gib, Disk: 300 * gib},
			fits:      true,
		},
		"one on no node fits anywhere it will be placed": {
			vm:        on("nowhere", "", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, time.Time{}, vmKind.Failed),
			resources: vmKind.Resources{CPUs: 64, Memory: 640 * gib, Disk: 10000 * gib},
			fits:      true,
		},
		"one on a node nobody knows is its node's to refuse": {
			vm:        on("lost", "gone", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, now, vmKind.Running),
			resources: vmKind.Resources{CPUs: 64, Memory: 640 * gib, Disk: 10000 * gib},
			fits:      true,
		},
		"and so is one on a node gone quiet": {
			nodes:     []node.Node{spoke("a", time.Hour, 16, 64, 1000, vm.Resources{})},
			vm:        on("quiet", "a", vmKind.Resources{CPUs: 2, Memory: 16 * gib, Disk: 100 * gib}, now, vmKind.Running),
			resources: vmKind.Resources{CPUs: 64, Memory: 640 * gib, Disk: 10000 * gib},
			fits:      true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fits, err := placementOf(t, tt.nodes, tt.vms...).Fits(ctx, tt.vm, tt.resources)
			require.NoError(t, err)
			assert.Equal(t, tt.fits, fits)
		})
	}
}

func TestPlacement_Alive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	p := placementOf(t, []node.Node{spoke("fresh", Fresh, 16, 64, 1000, vm.Resources{}), spoke("quiet", Fresh+time.Second, 16, 64, 1000, vm.Resources{})})

	for nodeName, alive := range map[string]bool{"fresh": true, "quiet": false, "unknown": false, "": false} {
		got, err := p.Alive(ctx, nodeName)
		require.NoError(t, err)
		assert.Equal(t, alive, got, nodeName)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	p := New(nodesMemory.NewRepository(), records.New(resourcesMemory.NewRepository()), 0.5)

	assert.Equal(t, 1.0, p.overcommit, "vcpus are never given less than once")
}
