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

// Registry is the set of runners currently connected.
//
// Connections arrive and go while requests are being routed, so an
// implementation has to be safe for concurrent use.
type Registry interface {
	// Exists reports whether a worker of that name is connected. A name is all
	// a worker is: it is unique across the cluster, it is what the certificate
	// says, and there is nothing else to look up -- so this answers the only
	// question there is to ask, rather than handing back the name it was given.
	//
	// false and a nil error is a worker that is not there. An error is this
	// being unable to say, which a registry kept anywhere but in memory can be
	// and which is not the same answer at all.
	Exists(ctx context.Context, name string) (bool, error)
}
