package stack

import (
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Want is a stack as a graph needs it: the root of a graph of its own, known
// by its record's uuid, of the person's, whose file's services, volumes and
// networks are each a container, a volume and a network its graph makes in
// the Docker VM VM names, each named under its slug (Named).
type Want struct {
	// UUID is its record's, and OwnerUUID the person it is the stack of.
	UUID      string `json:"uuid"`
	OwnerUUID string `json:"owner_uuid"`

	// Slug is what what it makes is named under, beside the name its file
	// gives each.
	Slug string `json:"slug"`

	// VM is the Docker VM it goes into: its uuid, and what to make it with
	// when it is not there (vm.Want), as what it makes in it wants it too.
	VM VMChoice `json:"vm"`

	// File is what its file says.
	File File `json:"file"`
}

// Requirement is the stack as a graph requires it, by its record's uuid:
// stack/<uuid>.
func (w Want) Requirement() graph.Requirement {
	return graph.Requirement{Kind: Name, Name: w.UUID, Spec: w}
}

// Named is the Docker name of what the stack makes of what its file calls
// name, a service, a volume or a network: <slug>_<name>.
func (w Want) Named(name string) string {
	return w.Slug + "_" + name
}
