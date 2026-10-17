package getResourceEndpoint

import "github.com/khanzadimahdi/testproject/domain/workload/port"

// Request asks where a port of the resource a slug names is reached.
type Request struct {
	// Kind is the resource's kind, as the route it was asked on names it.
	Kind string `json:"-"`

	// Slug is the name the resource is addressed by from outside.
	Slug string `json:"slug"`

	// Port is its own port that was asked for. Zero asks for the lowest one
	// it exposes, which is what a hostname naming no port means.
	Port port.Port `json:"port"`
}
