// Package network owns the docker network the workload puts standalone
// isolated tasks on.
package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/filters"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

// Manager owns the networks the workload puts tasks on.
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

// EnsureIsolatedNetwork creates the network standalone isolated tasks
// join, if it is not there already.
func (m *Manager) EnsureIsolatedNetwork(ctx context.Context) error {
	return m.ensure(ctx, network.IsolatedNetworkName)
}

// ensure creates the bridge tasks are isolated on, if one of that name is
// not there already.
//
// The isolation comes from having no masquerade rule rather than from docker's
// own internal flag. Both stop a task reaching the internet — without
// masquerading its packets leave carrying a private address and nothing comes
// back — but an internal network cannot have its ports published at all, and
// publishing them is the point. With masquerading off, the host still reaches
// the tasks on the bridge, so a published port works, while none of them can
// call out.
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
		// another orchestrator on the same daemon may have won the race, which
		// leaves exactly the network this was asking for.
		if _, inspectErr := m.client.NetworkInspect(ctx, name, networkTypes.InspectOptions{}); inspectErr == nil {
			return nil
		}

		return errors.Join(err, fmt.Errorf("failed to create the %q network", name))
	}

	m.logger.Info("network created", "network", name)

	return nil
}
