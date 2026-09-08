// Package network owns the docker networks the runner puts containers on: the
// shared one standalone isolated containers join, and the private one each
// stack gets so its services reach each other by name.
package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/filters"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
)

const (
	// detachTimeout is how long a stack's network is given to come free of the
	// containers being removed alongside it.
	detachTimeout = 30 * time.Second

	// detachInterval is how often it is tried in the meantime.
	detachInterval = time.Second
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
//
// The containers are removed on the strength of one message and the network on
// another, so the network is often still holding them when this is asked for.
// Docker will not remove a network anything is attached to, and a stack whose
// services are on their way out will be free within moments — so this waits for
// them rather than leaving the network behind for good.
func (m *Manager) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	name := network.StackNetworkName(stackSlug)

	deadline := time.Now().Add(detachTimeout)

	for attempt := 0; ; attempt++ {
		err := m.client.NetworkRemove(ctx, name)
		if err == nil {
			m.logger.Info("stack network removed", "network", name, "attempts", attempt+1)

			return nil
		}

		if client.IsErrNotFound(err) {
			return nil
		}

		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("the %q network still holds containers after %s: %w", name, detachTimeout, err)
		}

		select {
		case <-time.After(detachInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ensure creates the bridge containers are isolated on, if one of that name is
// not there already.
//
// The isolation comes from having no masquerade rule rather than from docker's
// own internal flag. Both stop a container reaching the internet — without
// masquerading its packets leave carrying a private address and nothing comes
// back — but an internal network cannot have its ports published at all, and
// publishing them is the point. With masquerading off, the host still reaches
// the containers on the bridge, so a published port works and the containers
// still reach each other, while none of them can call out.
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
		Driver:  "bridge",
		Options: map[string]string{"com.docker.network.bridge.enable_ip_masquerade": "false"},
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
