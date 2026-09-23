package firecracker

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Node says what a node's machines are using, all together.
type Node struct {
	runtime *Runtime
}

var _ node.Manager = &Node{}

func NewNode(runtime *Runtime) *Node {
	return &Node{runtime: runtime}
}

func (n *Node) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	ctx, span := n.runtime.tracer.Start(ctx, "firecracker.node.stats",
		oteltrace.WithAttributes(attribute.String("node.name", nodeName)),
	)
	defer span.End()

	var aggregate node.Stats

	count := 0
	for _, rec := range n.runtime.store.all() {
		if rec.Execution.NodeName != nodeName || !rec.running() {
			continue
		}

		s, err := n.runtime.Stats(ctx, rec.Execution.ID)
		if err != nil {
			return node.Stats{}, trace.RecordError(span, err)
		}

		count++

		aggregate.PIDs += s.PIDs
		aggregate.CPUPercent += s.CPUPercent
		aggregate.MemoryUsage += s.MemoryUsage
		aggregate.MemoryLimit += s.MemoryLimit
		aggregate.NetworkInput += s.NetworkInput
		aggregate.NetworkOutput += s.NetworkOutput
		aggregate.BlockInput += s.BlockInput
		aggregate.BlockOutput += s.BlockOutput
	}

	span.SetAttributes(attribute.Int("task.count", count))

	if aggregate.MemoryLimit > 0 {
		aggregate.MemoryPercent = float64(aggregate.MemoryUsage) / float64(aggregate.MemoryLimit) * 100.0
	}

	return aggregate, nil
}
