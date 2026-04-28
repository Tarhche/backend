package node

import (
	"context"
	"fmt"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type DockerManager struct {
	client *client.Client

	containerManager container.Manager
}

var _ node.Manager = &DockerManager{}

func NewDockerManager(dockerHost string, containerManager container.Manager) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerManager{client: cli, containerManager: containerManager}, nil
}

func (m *DockerManager) Stats(nodeName string) (node.Stats, error) {
	filter := filters.NewArgs()
	filter.Add("label", container.NodeNameLabelKey+"="+nodeName)
	filter.Add("status", "running")

	containers, err := m.client.ContainerList(context.Background(), containerTypes.ListOptions{Filters: filter})
	if err != nil {
		return node.Stats{}, err
	}

	var aggregate node.Stats
	for _, c := range containers {
		s, err := m.containerManager.Stats(c.ID)
		if err != nil {
			return node.Stats{}, err
		}

		aggregate.PIDs += s.PIDs
		aggregate.CPUPercent += s.CPUPercent
		aggregate.MemoryUsage += s.MemoryUsage
		aggregate.MemoryLimit += s.MemoryLimit
		aggregate.NetworkInput += s.NetworkInput
		aggregate.NetworkOutput += s.NetworkOutput
		aggregate.BlockInput += s.BlockInput
		aggregate.BlockOutput += s.BlockOutput
	}

	if aggregate.MemoryLimit > 0 {
		aggregate.MemoryPercent = float64(aggregate.MemoryUsage) / float64(aggregate.MemoryLimit) * 100.0
	}

	return aggregate, nil
}
