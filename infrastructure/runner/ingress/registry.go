// Package ingress is the tunnel told in the terms the rest of the application
// already has: which workers are connected, and therefore which ones can be
// reached.
//
// Nothing is recorded here. A worker is in the registry for exactly as long as
// its connections are open, so what this reads is the connections themselves
// rather than anything either side had to remember to say.
package ingress

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

// Connections is the part of the tunnel the registry needs: which agents are
// holding connections open. Asking for no more than this is what lets the
// registry be driven by a double in tests, and keeps the tunnel from having to
// know that an agent is what this application calls a worker.
type Connections interface {
	Agents() []tunnel.AgentState
}

// Registry answers whether a worker can be reached.
type Registry struct {
	connections Connections
}

// Ensure Registry implements ingress.Registry.
var _ ingress.Registry = &Registry{}

func NewRegistry(connections Connections) *Registry {
	return &Registry{connections: connections}
}

func (r *Registry) Exists(_ context.Context, name string) (bool, error) {
	for _, worker := range r.connections.Agents() {
		if worker.Name == name {
			return true, nil
		}
	}

	// it is not there, and this can say so for certain rather than reporting
	// that it could not find out: what it read is the connections themselves.
	return false, nil
}
