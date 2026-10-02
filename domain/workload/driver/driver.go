// Package driver is how an orchestrator runs tasks of more than one class.
//
// Each class a node offers is a driver: a kind of technology (a docker daemon,
// a vmhost) reached at an endpoint with options. The orchestrator's use cases
// ask one task.Runtime, network.Manager and node.Manager, as they always did;
// behind them a multiplexer sends each run's commands to the driver of its
// class, which a run's ID carries (runtime.Qualify). Only running a task needs
// to know which driver to ask, since that is where a class is chosen.
//
// It is a package of its own, rather than part of runtime, so that task can
// know about classes without knowing about drivers.
package driver

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// ErrUnknownClass is a class this node does not offer.
var ErrUnknownClass = errors.New("this node does not offer that runtime class")

// Kind is the technology behind a driver. Two classes can share one: gvisor
// would be the container kind pointed at a daemon with runsc installed.
type Kind string

const (
	// KindContainer runs a task as a container on a docker daemon.
	KindContainer Kind = "container"

	// KindMicroVM runs a task as a microVM, through vmhost.
	KindMicroVM Kind = "microvm"
)

func (k Kind) String() string {
	return string(k)
}

// Driver is one way of running tasks on this node.
//
// What it runs it acts on only when the run is labelled with this node's name,
// so that several orchestrators can share one docker daemon, or one vmhost,
// without acting on each other's runs.
type Driver interface {
	// Class is the class this driver runs, and Kind the technology it is.
	Class() runtime.Class
	Kind() Kind

	// Tasks are the runs of this class. Their IDs are the driver's own,
	// unqualified: putting the class in front is the multiplexer's job.
	Tasks() task.Runtime

	// Networks are the networks this class's runs join. A stack's network
	// belongs to the driver that made it.
	Networks() network.Manager

	// Node is what this class's runs use between them.
	Node() node.Manager

	// Offer is the class as this node runs it right now. It is asked for on
	// every node heartbeat, once a second, so it must be cheap: a driver
	// answers from what it already knows rather than asking anything slow.
	// A driver that cannot reach what stands behind it offers the class
	// unhealthy, with the reason, rather than failing.
	Offer(ctx context.Context) runtime.Offer

	// Close lets go of what the driver holds. What it runs carries on: a run
	// belongs to its daemon or its vmhost, not to the orchestrator.
	Close() error
}

// Set is every driver this node runs tasks with.
type Set interface {
	// For is the driver of a class, or ErrUnknownClass when this node does
	// not offer it.
	For(class runtime.Class) (Driver, error)

	// All is every driver, in the order they were configured.
	All() []Driver
}

// Factory builds the driver of one class, of the kind it is registered for.
// node is the orchestrator the driver runs for, which is the name its runs are
// labelled with. Anything else a driver needs — a logger, a tracer — is the
// factory's own, given when it was made.
type Factory func(ctx context.Context, node string, spec Spec) (Driver, error)
