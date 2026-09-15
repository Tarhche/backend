package network

import "context"

// Manager owns the networks the runner puts tasks on: the shared one that
// standalone isolated tasks join, and the private one each stack gets so
// its services reach each other by name.
type Manager interface {
	EnsureIsolatedNetwork(ctx context.Context) error
	EnsureStackNetwork(ctx context.Context, stackSlug string) error
	RemoveStackNetwork(ctx context.Context, stackSlug string) error
}
