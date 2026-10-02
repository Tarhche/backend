// Package node says what one class's runs on a node use between them.
package node

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Manager adds up what a node's running runs of one class use.
//
// It asks the class's own runtime for them, rather than its daemon, so that
// two classes on one daemon each count only their own: the runtime already
// knows which runs are its class's and which node's. A node with several
// classes is the sum of them, which is the multiplexer's to add.
type Manager struct {
	tasks  task.Runtime
	tracer oteltrace.Tracer
}

var _ node.Manager = &Manager{}

// NewManager is what the runs of tasks use on a node.
func NewManager(tasks task.Runtime) *Manager {
	return &Manager{tasks: tasks, tracer: otel.Tracer("docker")}
}

// Stats is the sum of what the node's running runs use. Memory is what they
// use against what they are limited to, so a node whose runs are near their
// own limits reads as full.
func (m *Manager) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	ctx, span := m.tracer.Start(ctx, "docker.node.stats",
		oteltrace.WithAttributes(attribute.String("node.name", nodeName)),
	)
	defer span.End()

	runs, err := m.tasks.OnNode(ctx, nodeName)
	if err != nil {
		return node.Stats{}, trace.RecordError(span, err)
	}

	var (
		aggregate node.Stats
		running   int
	)

	for _, run := range runs {
		// only what is running uses anything: an ended run is kept for its
		// logs and its exit code, and holds nothing.
		if run.Status != task.StatusRunning {
			continue
		}

		running++

		s, err := m.tasks.Stats(ctx, run.ID)
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

	span.SetAttributes(attribute.Int("task.count", running))

	if aggregate.MemoryLimit > 0 {
		aggregate.MemoryPercent = float64(aggregate.MemoryUsage) / float64(aggregate.MemoryLimit) * 100.0
	}

	return aggregate, nil
}
