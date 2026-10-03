// Package placement chooses the node a task is run on.
//
// It is a chain of filters in front of the scheduler the workload already had,
// rather than a scheduler of its own. A node is a candidate when it is speaking
// (healthy), when it offers the class the task is run with and that offer is
// healthy too, and when the class, as that node runs it, can do what the task
// asks of it: its network policy, a read-only root, its restart policy, its
// size, a network of its stack's own. The scheduler then picks among the
// candidates, as it always picked among every healthy node.
//
// What is left when no node survives decides what becomes of the task. A class
// that no node offers at all is not something waiting changes, and the task is
// told so (ErrNoNodeOffersRuntime). A class that some node offers, but none can
// run the task with right now — its node is not speaking, its driver is down,
// it cannot honour something the task asks for — is something waiting may
// change, and the task waits to be placed again (ErrNoNodeReady). Neither is
// for returning from a message handler: an error there is redelivered at once,
// and for ever.
//
// Room is not one of the filters yet (P4, known and accepted): a task is not
// turned away for want of memory here, and vmhost's admission refuses a VM that
// would not fit, which fails the task on its node instead.
package placement

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const (
	// NodesLimit is how many nodes are looked at. Only the first ten are
	// read, a known gap (P4) that matters once there are more of them.
	NodesLimit = 10

	// heartbeatGrace is how stale a node's last heartbeat may be before the
	// node is passed over. A node beats once a second, so this is a few
	// missed beats rather than one.
	heartbeatGrace = 3 * time.Second
)

var (
	// ErrNoNodeOffersRuntime is a class that no node the workload knows of
	// offers, healthy or not: there is nowhere for the task to wait for.
	ErrNoNodeOffersRuntime = errors.New("no node offers the runtime class")

	// ErrNoNodeReady is a class that nodes offer, none of which can run the
	// task right now.
	ErrNoNodeReady = errors.New("no node can run the task right now")
)

// Placement chooses nodes.
type Placement struct {
	nodeRepository node.Repository
	scheduler      task.Scheduler
	now            func() time.Time
}

func New(nodeRepository node.Repository, scheduler task.Scheduler) *Placement {
	return &Placement{
		nodeRepository: nodeRepository,
		scheduler:      scheduler,
		now:            time.Now,
	}
}

// Place chooses the node a task is run on.
func (p *Placement) Place(ctx context.Context, t *task.Task) (node.Node, error) {
	return p.place(ctx, t, t.Runtime.OrSysbox(), NeedOf(t))
}

// PlaceTogether chooses the one node several tasks of one class are run on,
// as a stack's services are: the network they share is local to the node that
// made it, so it has to be a node that can run every one of them.
func (p *Placement) PlaceTogether(ctx context.Context, class runtime.Class, needs ...Need) (node.Node, error) {
	// the scheduler is asked about a task, and these are not tasks yet: what
	// they share is their class.
	return p.place(ctx, &task.Task{Runtime: class.OrSysbox()}, class.OrSysbox(), needs...)
}

func (p *Placement) place(ctx context.Context, t *task.Task, class runtime.Class, needs ...Need) (node.Node, error) {
	nodes, err := p.nodeRepository.GetAll(ctx, 0, NodesLimit)
	if err != nil {
		return node.Node{}, err
	}

	now := p.now()

	// offered is whether any node the workload knows of offers the class at
	// all, speaking or not: one being redeployed is coming back, and what it
	// offers is worth waiting for.
	offered := false
	candidates := make([]node.Node, 0, len(nodes))

	for _, n := range nodes {
		offer, ok := OfferOf(n, class)
		if !ok {
			continue
		}

		offered = true

		if !Healthy(n, now) || !offer.Healthy || !metByAll(offer.Capabilities, needs) {
			continue
		}

		candidates = append(candidates, n)
	}

	switch {
	case len(candidates) > 0:
		return p.scheduler.Pick(t, candidates), nil

	// a platform none of whose nodes has spoken yet, as one that is only now
	// starting: nothing can be said about what they will offer.
	case offered || len(nodes) == 0:
		return node.Node{}, ErrNoNodeReady

	default:
		return node.Node{}, ErrNoNodeOffersRuntime
	}
}

// Need is what one task asks of the class it is run with, which the class, as
// a node runs it, has to be able to give.
type Need struct {
	NetworkPolicy network.Policy
	ReadOnly      bool
	RestartPolicy string

	// Memory is in bytes and CPU in cores, as a task's limits are.
	Memory uint64
	CPU    float64

	// StackNetwork says the task is a service of a stack, which reaches the
	// rest of it on a network of the stack's own.
	StackNetwork bool
}

// NeedOf is what a task asks of whatever runs it.
func NeedOf(t *task.Task) Need {
	return Need{
		NetworkPolicy: t.NetworkPolicy,
		ReadOnly:      t.ReadOnly,
		RestartPolicy: t.RestartPolicy,
		Memory:        t.ResourceLimits.Memory,
		CPU:           t.ResourceLimits.Cpu,
		StackNetwork:  len(t.StackUUID) > 0,
	}
}

// MetBy reports whether a class that can do this can give a task what it
// needs. A class declares what it enforces rather than what it accepts, so a
// task is never sent where something it asked for would be quietly dropped.
func (n Need) MetBy(c runtime.Capabilities) bool {
	policy := n.NetworkPolicy
	if len(policy) == 0 {
		policy = network.DefaultPolicy
	}

	switch {
	case !c.SupportsNetworkPolicy(policy):
		return false

	case n.ReadOnly && !c.ReadOnlyRoot:
		return false

	case !c.SupportsRestartPolicy(n.RestartPolicy):
		return false

	case n.StackNetwork && !c.StackNetworks:
		return false

	// zero is no bound.
	case c.MaxMemory > 0 && n.Memory > c.MaxMemory:
		return false

	case c.MaxCPU > 0 && n.CPU > c.MaxCPU:
		return false
	}

	return true
}

func metByAll(c runtime.Capabilities, needs []Need) bool {
	for _, need := range needs {
		if !need.MetBy(c) {
			return false
		}
	}

	return true
}
