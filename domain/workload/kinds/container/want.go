package container

import (
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// Want is a container as a graph needs it: one of the person's, in the
// Docker VM VM names, as Spec asks for it.
//
// One somebody asked for, which is a graph's root of its own, is known by its
// record's uuid; any other, a stack's, by its Docker name in its VM, which is
// what its node in a graph is named by too, when it has one.
type Want struct {
	// UUID is its record's, when it is known by it.
	UUID string `json:"uuid,omitempty"`

	// OwnerUUID is the person it is the container of, and Labels what its
	// record is labelled with.
	OwnerUUID string            `json:"owner_uuid"`
	Labels    map[string]string `json:"labels,omitempty"`

	// VM is the Docker VM it goes into: its uuid, and what to make it with
	// when it is not there (vm.Want). The spec's own VM is not read.
	VM stackKind.VMChoice `json:"vm"`

	// Spec is what it is asked for as: its Docker name, the networks it
	// joins, the volumes it mounts, and which of them are not its own
	// (Spec.External), among the rest.
	Spec Spec `json:"spec"`
}

// Requirement is the container as a graph requires it, by its Docker name,
// container/<name>, or by its record's uuid when it has none.
func (w Want) Requirement() graph.Requirement {
	name := w.Spec.Name
	if len(name) == 0 {
		name = w.UUID
	}

	return graph.Requirement{Kind: Name, Name: name, Spec: w}
}
