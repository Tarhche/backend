// Package placement chooses the node a VM lives on.
//
// A VM lives on one node for its whole life, because its disk is there, so the
// choice is made once and made by room: memory and disk are never given twice,
// while vCPUs may be, by the overcommit the control plane is configured with,
// because a VM rarely keeps all of its vCPUs busy.
//
// What a node has given away is what its engine said in its last heartbeat,
// plus what has been asked of it since that it has not reported yet. Without
// the second, two VMs asked for between two heartbeats would both see the room
// only one of them fits in.
package placement

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// Fresh is how recently a node must have spoken to be given a VM. Nodes
	// speak every second, so this is many missed heartbeats rather than one,
	// and a node that has been quiet for longer is not one to hand a VM to.
	Fresh = 30 * time.Second

	// nodesLimit is the most nodes looked at. A workload has a handful.
	nodesLimit = 100
)

// Placement chooses nodes for VMs.
type Placement struct {
	nodes node.Repository
	vms   vm.Repository

	// overcommit is how many times over a node's vCPUs may be given.
	overcommit float64

	now func() time.Time
}

func New(nodes node.Repository, vms vm.Repository, cpuOvercommit float64) *Placement {
	if cpuOvercommit < 1 {
		cpuOvercommit = 1
	}

	return &Placement{nodes: nodes, vms: vms, overcommit: cpuOvercommit, now: time.Now}
}

// Pick is the node with the most memory left once a VM given resources is on
// it, or vm.ErrNoCapacity when no node has room for one.
//
// Most memory first because memory is what runs out: it is never given twice
// and a VM holds all of it from the moment it boots. Ties go to the node with
// the most disk left, and then by name, so the same question gets the same
// answer.
func (p *Placement) Pick(ctx context.Context, resources vm.Resources) (node.Node, error) {
	nodes, err := p.nodes.GetAll(ctx, 0, nodesLimit)
	if err != nil {
		return node.Node{}, err
	}

	type candidate struct {
		node node.Node
		left vm.Resources
	}

	candidates := make([]candidate, 0, len(nodes))
	for _, n := range nodes {
		if !p.usable(n) {
			continue
		}

		allocated, err := p.allocated(ctx, n, "")
		if err != nil {
			return node.Node{}, err
		}

		if left, fits := p.fit(n.Capacity, allocated, resources); fits {
			candidates = append(candidates, candidate{node: n, left: left})
		}
	}

	if len(candidates) == 0 {
		return node.Node{}, vm.ErrNoCapacity
	}

	best := slices.MinFunc(candidates, func(a, b candidate) int {
		return cmp.Or(
			cmp.Compare(b.left.Memory, a.left.Memory),
			cmp.Compare(b.left.Disk, a.left.Disk),
			cmp.Compare(a.node.Name, b.node.Name),
		)
	})

	return best.node, nil
}

// Fits reports whether the node holding a VM has room for it to be given
// resources instead of what it has now.
//
// A node whose capacity is not known, or that has not spoken lately, is not
// refused here: the VM is already on it and nowhere else can take it, and the
// node refuses for itself when it has no room, which is reported back as the
// VM failing.
func (p *Placement) Fits(ctx context.Context, v *vm.VM, resources vm.Resources) (bool, error) {
	if len(v.NodeName) == 0 {
		return true, nil
	}

	n, err := p.nodes.GetOne(ctx, v.NodeName)
	if errors.Is(err, domain.ErrNotExists) {
		return true, nil
	} else if err != nil {
		return false, err
	}

	if !p.usable(n) {
		return true, nil
	}

	allocated, err := p.allocated(ctx, n, v.UUID)
	if err != nil {
		return false, err
	}

	// what the VM already has is counted by the engine once the node has
	// reported it, so it is taken back out before what it would have instead
	// is put in.
	if !pending(v) {
		allocated = subtract(allocated, v.Resources)
	}

	_, fits := p.fit(n.Capacity, allocated, resources)

	return fits, nil
}

// usable reports whether a node can be given a VM: it runs them, it has said
// what it offers, and it has said so lately.
func (p *Placement) usable(n node.Node) bool {
	if n.Role == node.ControlPlaneRole {
		return false
	}

	if n.Capacity.CPUs == 0 || n.Capacity.Memory == 0 || n.Capacity.Disk == 0 {
		return false
	}

	return p.now().Sub(n.LastHeartbeatAt) <= Fresh
}

// allocated is what a node has given away: what its engine last reported, and
// the VMs asked of it since that it has not reported yet. except leaves one VM
// out of the second, as though it were not there.
func (p *Placement) allocated(ctx context.Context, n node.Node, except string) (vm.Resources, error) {
	allocated := n.Capacity.Allocated

	held, err := p.vms.GetAllByNode(ctx, n.Name)
	if err != nil {
		return vm.Resources{}, err
	}

	for i := range held {
		if held[i].UUID == except || !pending(&held[i]) {
			continue
		}

		allocated = add(allocated, held[i].Resources)
	}

	return allocated, nil
}

// pending reports whether a VM was asked of its node and has not been reported
// by it yet, so that its engine has not counted it. One that failed before
// it got there holds nothing, and one on its way out is not coming.
func pending(v *vm.VM) bool {
	if !v.LastHeartbeatAt.IsZero() {
		return false
	}

	return v.CurrentState != vm.Failed && v.CurrentState != vm.Deleting
}

// fit reports whether resources fit in what a node offers beyond what it has
// given away, and what would be left if they went there.
func (p *Placement) fit(budget vm.Info, allocated vm.Resources, resources vm.Resources) (vm.Resources, bool) {
	if allocated.Memory+resources.Memory > budget.Memory {
		return vm.Resources{}, false
	}

	if allocated.Disk+resources.Disk > budget.Disk {
		return vm.Resources{}, false
	}

	if float64(allocated.CPUs+resources.CPUs) > float64(budget.CPUs)*p.overcommit {
		return vm.Resources{}, false
	}

	return vm.Resources{
		CPUs:   uint(float64(budget.CPUs)*p.overcommit) - allocated.CPUs - resources.CPUs,
		Memory: budget.Memory - allocated.Memory - resources.Memory,
		Disk:   budget.Disk - allocated.Disk - resources.Disk,
	}, true
}

func add(a vm.Resources, b vm.Resources) vm.Resources {
	return vm.Resources{CPUs: a.CPUs + b.CPUs, Memory: a.Memory + b.Memory, Disk: a.Disk + b.Disk}
}

// subtract takes b out of a, and never below nothing: an engine that has not
// counted a VM yet has nothing of it to give back.
func subtract(a vm.Resources, b vm.Resources) vm.Resources {
	return vm.Resources{
		CPUs:   a.CPUs - min(a.CPUs, b.CPUs),
		Memory: a.Memory - min(a.Memory, b.Memory),
		Disk:   a.Disk - min(a.Disk, b.Disk),
	}
}
