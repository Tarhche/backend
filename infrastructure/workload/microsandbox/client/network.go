package client

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

// NetworkManager is the workload's networks as microsandbox has them, which is
// not at all. A microVM's network is its own sandbox's, made with it and
// policed by its policy, and there is nothing shared to make beforehand or take
// away afterwards.
type NetworkManager struct{}

var _ network.Manager = &NetworkManager{}

func NewNetworkManager() *NetworkManager {
	return &NetworkManager{}
}

// EnsureIsolatedNetwork has nothing to make: an isolated run is isolated by its
// own policy, set when it is created. Unlike docker's shared isolated network,
// isolated runs do not reach each other by name.
func (m *NetworkManager) EnsureIsolatedNetwork(ctx context.Context) error {
	return nil
}

// EnsureStackNetwork refuses, since microsandbox has no network between
// sandboxes for a stack's services to reach each other on. The refusal is the
// stack's failure reason, before any of its services is created.
func (m *NetworkManager) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return ErrStack
}

// RemoveStackNetwork has nothing to remove, since no stack's network was ever
// made.
func (m *NetworkManager) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	return nil
}
