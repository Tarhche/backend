// Package ingress is the runner cluster as the ingress knows it: which workers
// are connected, and therefore which ones can be reached.
//
// Nothing here says where a worker is, because nothing dials one. A worker
// opens connections to the ingress and says who it is; the ingress keeps them
// and sends requests back down them. Being reachable and being alive are then
// the same fact, and it is the connection itself rather than anything either
// side has to remember.
package ingress

import (
	"context"
	"time"
)

// Runner is a worker the ingress can reach: the id it is addressed by, which is
// the worker's own name, and how much of it is currently connected.
type Runner struct {
	ID string

	// Connections is how many of this runner's connections the ingress is
	// holding. One is enough to be reachable; the worker decides how many more
	// to keep ready.
	Connections uint

	// ConnectedAt is when the first of them arrived, which is as close to "when
	// this worker came up" as the ingress can see.
	ConnectedAt time.Time
}

// Registry is the set of runners currently connected.
//
// Connections arrive and go while requests are being routed, so an
// implementation has to be safe for concurrent use.
type Registry interface {
	// Get returns the runner an id names. It reports domain.ErrNotExists for an
	// id that has never connected, and for one whose connections have all gone.
	Get(ctx context.Context, id string) (Runner, error)

	// All returns every connected runner, ordered by id.
	All(ctx context.Context) ([]Runner, error)
}
