// Package ingress is the runner cluster as the ingress knows it: which workers
// are connected, and therefore which ones can be reached.
//
// Nothing here says where a worker is, because nothing dials one. A worker
// opens connections to the ingress and says who it is; the ingress keeps them
// and sends requests back down them. Being reachable and being alive are then
// the same fact, and it is the connection itself rather than anything either
// side has to remember.
package ingress

import "context"

// Runner is a worker the ingress can reach, which is its name and nothing else:
// how it is reached is the tunnel's business, and how much of it is connected
// is a question nothing asks.
type Runner struct {
	ID string
}

// Registry is the set of runners currently connected.
//
// Connections arrive and go while requests are being routed, so an
// implementation has to be safe for concurrent use.
type Registry interface {
	// Get returns the runner an id names. It reports domain.ErrNotExists for an
	// id that has never connected, and for one whose connections have all gone.
	Get(ctx context.Context, id string) (Runner, error)
}
