// Package getRuntimes says which classes the workload runs tasks with, and how.
//
// For each class a task may ask for, in the order the platform names them, it
// says whether the class is the default, how many nodes can run it right now,
// what all of those can do and what they hold between them. What all of them can
// do, rather than what any of them can, because a task of the class may be put
// on any of them: a form built from it never offers what a task would only get
// on some nodes. A class no node can run right now is listed all the same,
// unavailable, so that whoever is choosing sees why it cannot be chosen rather
// than not seeing it at all.
//
// It reads the nodes the way placement does, so a class it says is available
// is one a task would be placed with.
package getRuntimes

import (
	"context"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

type UseCase struct {
	nodeRepository node.Repository
	classes        allowed.Classes
	now            func() time.Time
}

func NewUseCase(nodeRepository node.Repository, classes allowed.Classes) *UseCase {
	return &UseCase{
		nodeRepository: nodeRepository,
		classes:        classes,
		now:            time.Now,
	}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	nodes, err := uc.nodeRepository.GetAll(ctx, 0, placement.NodesLimit)
	if err != nil {
		return nil, err
	}

	now := uc.now()

	classes := uc.classes.All()
	items := make([]runtime.Availability, len(classes))

	for i, class := range classes {
		items[i] = uc.availability(class, nodes, now)
	}

	return &Response{Items: items}, nil
}

// availability is one class across every node that can run it right now: a
// node that is speaking, whose offer of the class is healthy.
func (uc *UseCase) availability(class runtime.Class, nodes []node.Node, now time.Time) runtime.Availability {
	availability := runtime.Availability{
		Class:   class,
		Default: class == uc.classes.Default(),
	}

	for _, n := range nodes {
		if !placement.Healthy(n, now) {
			continue
		}

		offer, offered := placement.OfferOf(n, class)
		if !offered || !offer.Healthy {
			continue
		}

		if availability.Nodes == 0 {
			availability.Capabilities = clone(offer.Capabilities)
		} else {
			availability.Capabilities = common(availability.Capabilities, offer.Capabilities)
		}

		availability.Capacity = availability.Capacity.Add(offer.Capacity)
		availability.Nodes++
	}

	availability.Available = availability.Nodes > 0
	availability.Capabilities = lists(availability.Capabilities)

	return availability
}

// common is what two nodes running a class can both do, which is what a task
// placed on either of them can count on.
func common(a, b runtime.Capabilities) runtime.Capabilities {
	isolation := a.Isolation
	if a.Isolation != b.Isolation {
		isolation = ""
	}

	return runtime.Capabilities{
		Isolation:       isolation,
		NetworkPolicies: intersect(a.NetworkPolicies, b.NetworkPolicies),
		StackNetworks:   a.StackNetworks && b.StackNetworks,
		ReadOnlyRoot:    a.ReadOnlyRoot && b.ReadOnlyRoot,
		DiskLimit:       a.DiskLimit && b.DiskLimit,
		TTY:             a.TTY && b.TTY,
		RestartPolicies: intersect(a.RestartPolicies, b.RestartPolicies),

		// the least a task may ask for is the most any of them asks, and
		// the most it may ask for the least any of them allows.
		MinMemory: max(a.MinMemory, b.MinMemory),
		MaxMemory: tighter(a.MaxMemory, b.MaxMemory),
		MaxCPU:    tighter(a.MaxCPU, b.MaxCPU),

		Architectures: intersect(a.Architectures, b.Architectures),
	}
}

// intersect is what both lists hold, each once, in the order the first holds
// it.
func intersect[T comparable](a, b []T) []T {
	both := make([]T, 0, min(len(a), len(b)))

	for _, item := range a {
		if slices.Contains(b, item) && !slices.Contains(both, item) {
			both = append(both, item)
		}
	}

	return both
}

// tighter is the smaller of two bounds, where zero is no bound at all.
func tighter[T uint64 | float64](a, b T) T {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	default:
		return min(a, b)
	}
}

// clone is a node's capabilities, copied, so what is put together from them
// never writes into what the node said.
func clone(c runtime.Capabilities) runtime.Capabilities {
	c.NetworkPolicies = slices.Clone(c.NetworkPolicies)
	c.RestartPolicies = slices.Clone(c.RestartPolicies)
	c.Architectures = slices.Clone(c.Architectures)

	return c
}

// lists makes every list of a class's capabilities a list, empty rather than
// absent: a class nothing can run right now can do nothing, which is not the
// same as nobody having said.
func lists(c runtime.Capabilities) runtime.Capabilities {
	if c.NetworkPolicies == nil {
		c.NetworkPolicies = []network.Policy{}
	}

	if c.RestartPolicies == nil {
		c.RestartPolicies = []string{}
	}

	if c.Architectures == nil {
		c.Architectures = []string{}
	}

	return c
}
