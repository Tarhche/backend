// Package network is the network kind's node strategy: what a node does to
// the docker networks in its Docker VMs, and what it says they are.
//
// A network the platform made is labelled as the resource it is, and found by
// that label, or by its Docker id; one a stack's compose made, or a VM's
// terminal, is reported as it is, for the control plane to show beside its
// records.
package network

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
)

// Node is the network kind's node strategy.
type Node struct {
	reader *blocks.Reader

	// timeout bounds a command: waiting for its VM's dockerd, and the
	// command.
	timeout time.Duration
}

var _ kind.Node[networkKind.Spec, networkKind.Status] = &Node{}

// New is the strategy that reaches and reads Docker VMs through reader, each
// command given timeout.
func New(reader *blocks.Reader, timeout time.Duration) *Node {
	return &Node{reader: reader, timeout: timeout}
}

// Execute makes a network in its VM, or removes it from there.
func (n *Node) Execute(ctx context.Context, network networkKind.Network, action string, _ any) (kind.Outcome[networkKind.Status], error) {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	daemon, err := n.reader.Reach(ctx, networkKind.VMOf(network))

	switch {
	case errors.Is(err, blocks.ErrGone) && action == networkKind.ActionDelete:
		return kind.Outcome[networkKind.Status]{}, nil
	case err != nil:
		return failed(err)
	}

	found, there, err := find(ctx, daemon, network)
	if err != nil {
		return failed(err)
	}

	switch action {
	case networkKind.ActionCreate:
		// one made already, by a create asked again, is what was asked for.
		if !there {
			if found, err = daemon.CreateNetwork(ctx, network.Spec.Docker(network.Metadata.UUID)); err != nil {
				return failed(err)
			}
		}

		return kind.Outcome[networkKind.Status]{Status: networkKind.Status{
			Status:  kind.Status{State: networkKind.Present},
			Docker:  networkKind.DockerOf(found),
			Failure: blocks.Cleared(),
		}}, nil

	case networkKind.ActionDelete:
		if !there {
			return kind.Outcome[networkKind.Status]{}, nil
		}

		err := daemon.RemoveNetwork(ctx, found.ID)

		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return kind.Outcome[networkKind.Status]{}, nil

		// one docker would not remove, with a container on it say, is left
		// as it was.
		case errors.Is(err, docker.ErrInvalid):
			return kind.Outcome[networkKind.Status]{Status: networkKind.Status{
				Status:  kind.Status{State: networkKind.Present},
				Docker:  networkKind.DockerOf(found),
				Failure: blocks.Failure(err),
			}}, blocks.Refused(err)
		}

		return failed(err)
	}

	return failed(fmt.Errorf("%w: a network cannot be %s on its node", kind.ErrUnknownAction, action))
}

// Query answers none of a network's queries but its state, which is read
// off State.
func (n *Node) Query(_ context.Context, _ networkKind.Network, action string, _ any) (any, error) {
	return nil, fmt.Errorf("%w: a network has no %q", kind.ErrUnknownAction, action)
}

// State is every network of every running Docker VM on the node.
func (n *Node) State(ctx context.Context) (kind.Report[networkKind.Status], error) {
	read, err := n.reader.Read(ctx)
	if err != nil {
		return kind.Report[networkKind.Status]{}, err
	}

	report := blocks.Report[networkKind.Status](read)

	for _, vmUUID := range read.VMs() {
		inventory := read.Inventories[vmUUID]
		stacks := blocks.Stacks(inventory)

		for _, held := range inventory.Networks {
			report.Instances = append(report.Instances, kind.Observed[networkKind.Status]{
				UUID:   blockKinds.Identity(networkKind.Name, held.Labels),
				Owners: blocks.Owners(vmUUID, held.Labels, stacks),
				Status: networkKind.Status{Status: kind.Status{State: networkKind.Present}, Docker: networkKind.DockerOf(held)},
			})
		}
	}

	slices.SortFunc(report.Instances, func(a, b kind.Observed[networkKind.Status]) int {
		return cmp.Or(strings.Compare(a.UUID, b.UUID), strings.Compare(a.Status.Docker.ID, b.Status.Docker.ID))
	})

	return report, nil
}

// find is the network a resource is in its VM's dockerd: the one labelled as
// the resource, or the one its Docker id names, and whether there is one.
func find(ctx context.Context, daemon docker.Daemon, network networkKind.Network) (docker.Network, bool, error) {
	held, err := daemon.Networks(ctx)
	if err != nil {
		return docker.Network{}, false, err
	}

	for _, n := range held {
		if blockKinds.Identity(networkKind.Name, n.Labels) == network.Metadata.UUID {
			return n, true, nil
		}
	}

	if seen := network.Status.Docker; seen != nil && len(seen.ID) > 0 {
		for _, n := range held {
			if n.ID == seen.ID {
				return n, true, nil
			}
		}
	}

	return docker.Network{}, false, nil
}

// failed is what a command that failed left a network as: failed, saying why
// in the codes every side knows.
func failed(err error) (kind.Outcome[networkKind.Status], error) {
	return kind.Outcome[networkKind.Status]{Status: networkKind.Status{
		Status:  kind.Status{State: networkKind.Failed, Reason: err.Error()},
		Failure: blocks.Failure(err),
	}}, err
}
