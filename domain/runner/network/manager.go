package network

import "context"

// Manager owns the networks the runner puts containers on: the shared one that
// standalone isolated containers join, and the private one each stack gets so
// its services reach each other by name.
type Manager interface {
	EnsureIsolatedNetwork(ctx context.Context) error
	EnsureStackNetwork(ctx context.Context, stackSlug string) error
	RemoveStackNetwork(ctx context.Context, stackSlug string) error
}
