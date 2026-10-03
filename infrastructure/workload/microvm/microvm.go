// Package microvm is the orchestrator's driver for the microvm kind: a client
// of vmhost on the same host, which runs a task as a microVM of its own.
//
// It holds no privilege and keeps no state. Everything it knows about a run it
// reads back from vmhost, which keeps the run's labels as docker keeps a
// container's; so an orchestrator that is redeployed — which happens with
// every release — finds every VM where it left it, and nothing has to be
// handed over. It acts only on VMs labelled with its own node's name.
//
// A VM's ports are published nowhere: the driver reaches them through vmhost,
// which reaches them through the agent inside the VM, so it is a task.Dialer
// and the orchestrator's proxy dials through it.
package microvm

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// errNotImplemented is what a stub answers.
var errNotImplemented = errors.New("microvm driver: not implemented yet")

// Driver is one class of microVMs on this node.
type Driver struct {
	class  workloadRuntime.Class
	node   string
	client *Client
	logger *slog.Logger
}

var _ driver.Driver = (*Driver)(nil)

// New builds the driver of one class, reaching vmhost at the spec's endpoint
// (vm.DefaultEndpoint, as a rule).
func New(ctx context.Context, nodeName string, spec driver.Spec, logger *slog.Logger) (*Driver, error) {
	return nil, errNotImplemented
}

// NewFactory is how the registry builds drivers of the microvm kind.
func NewFactory(logger *slog.Logger) driver.Factory {
	return func(ctx context.Context, nodeName string, spec driver.Spec) (driver.Driver, error) {
		built, err := New(ctx, nodeName, spec, logger)
		if err != nil {
			return nil, err
		}

		return built, nil
	}
}

func (d *Driver) Class() workloadRuntime.Class {
	return d.class
}

func (d *Driver) Kind() driver.Kind {
	return driver.KindMicroVM
}

func (d *Driver) Tasks() task.Runtime {
	return &Runtime{client: d.client, node: d.node}
}

func (d *Driver) Networks() network.Manager {
	return &Networks{client: d.client}
}

func (d *Driver) Node() node.Manager {
	return &Node{client: d.client}
}

// Offer is the class as vmhost says it runs it, or unhealthy, with the reason,
// when vmhost cannot be reached.
func (d *Driver) Offer(ctx context.Context) workloadRuntime.Offer {
	return workloadRuntime.Offer{
		Class:  d.class,
		Driver: driver.KindMicroVM.String(),
		Reason: errNotImplemented.Error(),
	}
}

func (d *Driver) Close() error {
	return nil
}
