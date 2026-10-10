package vm

import (
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// Want is a VM as a graph needs it: the Docker VM a container, a network, a
// volume or a stack goes into, named by the uuid what needs it holds for it,
// whether or not it is there yet. It is made under that uuid whenever it is
// not there, with the Docker defaults and whatever else VM gives, and every
// requirement of one VM says all of it, so that whichever of them a graph
// keeps can make it.
type Want struct {
	// OwnerUUID is the person it is made for.
	OwnerUUID string `json:"owner_uuid"`

	// VM is its uuid and, beside it, what of a name, resources, ports and a
	// network it is made with beyond the Docker defaults: what a container's
	// or a stack's own choice of VM refuses as asking for two VMs is one VM
	// here, and how to make it again.
	VM stackKind.VMChoice `json:"vm"`
}

// Requirement is the VM as a graph requires it, by its uuid: vm/<uuid>.
func (w Want) Requirement() graph.Requirement {
	return graph.Requirement{Kind: Name, Name: w.VM.UUID, Spec: w}
}
