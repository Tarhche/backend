package network

import "context"

// Manager owns the network the workload puts standalone isolated tasks on.
type Manager interface {
	EnsureIsolatedNetwork(ctx context.Context) error
}
