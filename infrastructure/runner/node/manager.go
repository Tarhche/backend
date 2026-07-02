package node

import (
	"context"
	"fmt"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type DockerManager struct {
	client *client.Client
	tracer oteltrace.Tracer

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

	return &DockerManager{client: cli, containerManager: containerManager, tracer: otel.Tracer("docker")}, nil
}

func (m *DockerManager) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	ctx, span := m.tracer.Start(ctx, "docker.node.stats",
		oteltrace.WithAttributes(attribute.String("node.name", nodeName)),
	)
	defer span.End()

	filter := filters.NewArgs()
	filter.Add("label", container.NodeNameLabelKey+"="+nodeName)
	filter.Add("status", "running")

	containers, err := m.client.ContainerList(ctx, containerTypes.ListOptions{Filters: filter})
	if err != nil {
		return node.Stats{}, trace.RecordError(span, err)
	}

	span.SetAttributes(attribute.Int("container.count", len(containers)))

	var aggregate node.Stats
	for _, c := range containers {
		s, err := m.containerManager.Stats(ctx, c.ID)
		if err != nil {
			return node.Stats{}, trace.RecordError(span, err)
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
