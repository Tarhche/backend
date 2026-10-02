package microvm

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Node is what a class's VMs on this node use between them, as the container
// driver sums what its containers use: the dashboard's view of a node, which
// is not what placement reads. How much room a class has is its offer's to
// say (Driver.Offer).
type Node struct {
	client *Client
	node   string
	tracer oteltrace.Tracer
}

var _ node.Manager = (*Node)(nil)

// Stats is what this node's running VMs use together. Another node's VMs are
// not this driver's to count, whatever vmhost holds.
func (n *Node) Stats(ctx context.Context, nodeName string) (node.Stats, error) {
	if nodeName != n.node {
		return node.Stats{}, nil
	}

	ctx, span := n.tracer.Start(ctx, "microvm.node.stats",
		oteltrace.WithAttributes(attribute.String("node.name", nodeName)),
	)
	defer span.End()

	filter := nodeNameLabel + "=" + nodeName

	vms, err := n.client.VMs(ctx, filter)
	if err != nil {
		return node.Stats{}, trace.RecordError(span, err)
	}

	var aggregate node.Stats
	var counted int

	for _, v := range vms {
		if v.State != vm.StateRunning || !v.Matches([]string{filter}) {
			continue
		}

		used, err := n.client.Stats(ctx, v.ID)

		// one that ended, or went, since it was listed uses nothing any more,
		// which is no reason not to count the rest.
		if errors.Is(err, vm.ErrNotRunning) || errors.Is(err, vm.ErrNotFound) {
			continue
		}

		if err != nil {
			return node.Stats{}, trace.RecordError(span, err)
		}

		counted++

		aggregate.PIDs += used.PIDs
		aggregate.CPUPercent += used.CPUPercent
		aggregate.MemoryUsage += used.MemoryUsage
		aggregate.MemoryLimit += used.MemoryLimit
		aggregate.NetworkInput += used.NetworkInput
		aggregate.NetworkOutput += used.NetworkOutput
		aggregate.BlockInput += used.BlockInput
		aggregate.BlockOutput += used.BlockOutput
	}

	span.SetAttributes(attribute.Int("task.count", counted))

	if aggregate.MemoryLimit > 0 {
		aggregate.MemoryPercent = float64(aggregate.MemoryUsage) / float64(aggregate.MemoryLimit) * 100.0
	}

	return aggregate, nil
}
