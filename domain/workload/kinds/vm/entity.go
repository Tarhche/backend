package vm

import (
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// states are the vm package's states by their words, which are the kind's.
var states = func() map[kind.State]vm.State {
	words := make(map[kind.State]vm.State)

	for state := vm.Created; state <= vm.Deleting; state++ {
		words[kind.State(state.String())] = state
	}

	return words
}()

// StateOf is a state of the kind as the vm package has it, which is how the
// dashboard has always shown a VM: a VM expected deleted, and one deleted,
// are deleting. A state the vm package has no word for is none.
func StateOf(s kind.State) vm.State {
	if s == Deleted {
		return vm.Deleting
	}

	return states[s]
}

// Entity is a VM's manifest as the vm package's VM, which is what the
// dashboard shows a VM as, and what the parts of the workload not on the
// framework yet read one as: its spec, its status in the words a VM has
// always had, and when its node last spoke for it, which is when it was last
// observed.
func Entity(v VM) vm.VM {
	entity := vm.VM{
		UUID:           v.Metadata.UUID,
		Name:           v.Metadata.Name,
		Slug:           v.Metadata.Slug,
		OwnerUUID:      v.Metadata.OwnerUUID,
		Kind:           v.Spec.Flavor,
		Image:          v.Spec.Image,
		Resources:      v.Spec.Resources.VM(),
		Ports:          slices.Clone(v.Spec.Ports),
		Network:        v.Spec.Network.VM(),
		PersistentDisk: v.Spec.PersistentDisk,

		Lifetime:  v.Metadata.Lifetime,
		ExpiresAt: v.Metadata.ExpiresAt,

		CurrentState:  StateOf(v.Status.State),
		ExpectedState: StateOf(v.Status.Expected),
		Reason:        v.Status.Reason,
		NodeName:      v.Metadata.Node,
		Stats:         v.Status.Stats.VM(),

		LastHeartbeatAt: v.Status.ObservedAt,
		CreatedAt:       v.Metadata.CreatedAt,
		StartedAt:       v.Status.StartedAt,
		UpdatedAt:       v.Metadata.UpdatedAt,

		ManagedBy: v.Metadata.Labels[LabelManagedBy],
	}

	if entity.Ports == nil {
		entity.Ports = []port.Port{}
	}

	return entity
}
