package vm

import (
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
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

// ChoiceOf is v as what goes into it keeps it (stackKind.VMChoice): its uuid,
// and what it is made with again under that uuid whenever it is gone, its
// name, resources, ports and network as its record has them, and the slug it
// is reached under, once it has one. Ports that are none are kept as none,
// never as the defaults.
func ChoiceOf(v VM) stackKind.VMChoice {
	return stackKind.VMChoice{
		UUID:      v.Metadata.UUID,
		Name:      v.Metadata.Name,
		Resources: &stackKind.Resources{CPUs: v.Spec.Resources.CPUs, Memory: v.Spec.Resources.Memory, Disk: v.Spec.Resources.Disk},
		Ports:     append([]port.Port{}, v.Spec.Ports...),
		Network:   &stackKind.Network{Ingress: string(v.Spec.Network.Ingress), Egress: string(v.Spec.Network.Egress)},
		Slug:      v.Metadata.Slug,
	}
}
