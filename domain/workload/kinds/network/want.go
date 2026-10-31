package network

import (
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// Want is a network as a graph needs it: one of the person's, in the Docker
// VM VM names, as Spec asks for it, which its VM knows it by its name.
type Want struct {
	// OwnerUUID is the person it is the network of, and Labels what its
	// record is labelled with.
	OwnerUUID string            `json:"owner_uuid"`
	Labels    map[string]string `json:"labels,omitempty"`

	// VM is the Docker VM it is made in: its uuid, and what to make it with
	// when it is not there (vm.Want).
	VM stackKind.VMChoice `json:"vm"`

	Spec Spec `json:"spec"`
}

// Requirement is the network as a graph requires it, by its Docker name:
// network/<name>.
func (w Want) Requirement() graph.Requirement {
	return graph.Requirement{Kind: Name, Name: w.Spec.Name, Spec: w}
}
