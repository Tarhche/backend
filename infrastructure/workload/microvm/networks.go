package microvm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

const (
	// detachTimeout is how long a stack's network is given to come free of
	// the VMs being removed alongside it, as the docker manager gives a
	// stack's bridge.
	detachTimeout = 30 * time.Second

	// detachInterval is how often it is tried in the meantime.
	detachInterval = time.Second
)

// Networks are a class's VM networks, which vmhost makes: the one standalone
// isolated tasks share, and one for each stack, on which its services reach
// each other by name. None of them routes out; a task that may reach the
// internet joins vmhost's public network beside its own, which vmhost made
// when it started.
type Networks struct {
	client *Client
	logger *slog.Logger
	tracer oteltrace.Tracer

	// detachTimeout and detachInterval are how long, and how often, a
	// network that still holds VMs is asked to go.
	detachTimeout  time.Duration
	detachInterval time.Duration
}

var _ network.Manager = (*Networks)(nil)

// EnsureIsolatedNetwork makes the network standalone isolated tasks join, if
// it is not there already.
func (n *Networks) EnsureIsolatedNetwork(ctx context.Context) error {
	return n.ensure(ctx, network.IsolatedNetworkName)
}

// EnsureStackNetwork makes the network a stack's services share. Every
// service of a stack runs on one node, so it is this node's vmhost's alone.
func (n *Networks) EnsureStackNetwork(ctx context.Context, stackSlug string) error {
	return n.ensure(ctx, network.StackNetworkName(stackSlug))
}

// ensure makes a network that does not route out, if it is not there already.
// vmhost is asked every time: one that restarted on a host that rebooted has
// lost its bridges, and asking is cheap.
func (n *Networks) ensure(ctx context.Context, name string) error {
	ctx, span := n.tracer.Start(ctx, "microvm.network.ensure",
		oteltrace.WithAttributes(attribute.String("network", name)),
	)
	defer span.End()

	if _, err := n.client.EnsureNetwork(ctx, networkName(name), false); err != nil {
		return trace.RecordError(span, err)
	}

	return nil
}

// RemoveStackNetwork takes a stack's network away once its VMs are off it. A
// network that is not there is the outcome asked for.
//
// The VMs are removed on the strength of one message and the network on
// another, so the network is often still holding them when this is asked for.
// vmhost will not take away a network anything is plugged into, and a stack
// whose services are on their way out is free within moments, so this waits
// for them rather than leaving the network behind for good.
func (n *Networks) RemoveStackNetwork(ctx context.Context, stackSlug string) error {
	name := network.StackNetworkName(stackSlug)

	ctx, span := n.tracer.Start(ctx, "microvm.network.remove",
		oteltrace.WithAttributes(attribute.String("network", name)),
	)
	defer span.End()

	deadline := time.Now().Add(n.detachTimeout)

	for attempt := 1; ; attempt++ {
		err := n.client.RemoveNetwork(ctx, networkName(name))

		switch {
		case err == nil:
			n.logger.Info("stack network removed", "network", name, "attempts", attempt)

			return nil
		case errors.Is(err, vm.ErrNotFound):
			return nil
		case !errors.Is(err, vm.ErrNetworkInUse):
			return trace.RecordError(span, err)
		case time.Now().After(deadline):
			return trace.RecordError(span, fmt.Errorf("the %q network still holds vms after %s: %w", name, n.detachTimeout, err))
		}

		select {
		case <-time.After(n.detachInterval):
		case <-ctx.Done():
			return trace.RecordError(span, ctx.Err())
		}
	}
}
