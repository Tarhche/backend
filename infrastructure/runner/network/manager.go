// Package network owns the docker networks the runner puts containers on: the
// shared one standalone isolated containers join, and the private one each
// stack gets so its services reach each other by name.
package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/filters"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
)

// Manager owns the networks the runner puts containers on.
type Manager struct {
	client *client.Client
	logger *slog.Logger
}

func NewManager(dockerHost string, logger *slog.Logger) (*Manager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Manager{client: cli, logger: logger}, nil
}

// EnsureIsolatedNetwork creates the network standalone isolated containers
// join, if it is not there already.
func (m *Manager) EnsureIsolatedNetwork(ctx context.Context) error {
	return m.ensure(ctx, network.IsolatedNetworkName)
}

// EnsureStackNetwork creates the private network a stack's services share.
// Every service of a stack runs on one node, so the network is local to that
// node and a plain bridge is all it takes.
func (m *Manager) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return m.ensure(ctx, network.StackNetworkName(stackSlug))
}

// RemoveStackNetwork drops a stack's private network once its containers are
// gone. A network that is not there is the outcome asked for.
func (m *Manager) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	name := network.StackNetworkName(stackSlug)

	if err := m.client.NetworkRemove(ctx, name); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}

		return err
	}

	m.logger.Info("stack network removed", "network", name)

	return nil
}

// ensure creates an internal bridge, if one of that name is not there already.
// It is internal, so containers on it reach each other and the runner still
// publishes their ports — the host is attached to the bridge — while nothing on
// it can route out. A container that is meant to reach the internet joins the
// default bridge as well.
func (m *Manager) ensure(ctx context.Context, name string) error {
	existing, err := m.client.NetworkList(ctx, networkTypes.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return err
	}

	for _, n := range existing {
		if n.Name == name {
			return nil
		}
	}

	if _, err := m.client.NetworkCreate(ctx, name, networkTypes.CreateOptions{
		Driver:   "bridge",
		Internal: true,
	}); err != nil {
		// another worker on the same daemon may have won the race, which
		// leaves exactly the network this was asking for.
		if _, inspectErr := m.client.NetworkInspect(ctx, name, networkTypes.InspectOptions{}); inspectErr == nil {
			return nil
		}

		return errors.Join(err, fmt.Errorf("failed to create the %q network", name))
	}

	m.logger.Info("network created", "network", name)

	return nil
}
