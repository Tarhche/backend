// Package volume is the volume kind's node strategy: what a node does to the
// docker volumes in its Docker VMs, and what it says they are.
//
// A volume the platform made is labelled as the resource it is, and found by
// that label; one a stack's compose made, or a VM's terminal, is reported as
// it is, for the control plane to show beside its records. A volume made
// again after it went missing is labelled with when it was, and says that
// what was in it is gone for as long as it is there.
package volume

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
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

// Node is the volume kind's node strategy.
type Node struct {
	reader *blocks.Reader

	// timeout bounds a command: waiting for its VM's dockerd, and the
	// command.
	timeout time.Duration

	now func() time.Time
}

var _ kind.Node[volumeKind.Spec, volumeKind.Status] = &Node{}

// New is the strategy that reaches and reads Docker VMs through reader, each
// command given timeout.
func New(reader *blocks.Reader, timeout time.Duration) *Node {
	return &Node{reader: reader, timeout: timeout, now: time.Now}
}

// Execute makes a volume in its VM, or removes it from there.
func (n *Node) Execute(ctx context.Context, v volumeKind.Volume, action string, _ any) (kind.Outcome[volumeKind.Status], error) {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	daemon, err := n.reader.Reach(ctx, volumeKind.VMOf(v))

	switch {
	case errors.Is(err, blocks.ErrGone) && action == volumeKind.ActionDelete:
		return kind.Outcome[volumeKind.Status]{}, nil
	case err != nil:
		return failed(err)
	}

	held, err := daemon.Volumes(ctx)
	if err != nil {
		return failed(err)
	}

	found, there := find(held, v)

	switch action {
	case volumeKind.ActionCreate:
		if !there {
			if found, err = n.create(ctx, daemon, held, v); err != nil {
				return failed(err)
			}
		}

		return kind.Outcome[volumeKind.Status]{Status: volumeKind.Status{
			Status:  kind.Status{State: volumeKind.Present, Reason: volumeKind.ReasonOf(found.Labels)},
			Docker:  volumeKind.DockerOf(found),
			Failure: blocks.Cleared(),
		}}, nil

	case volumeKind.ActionDelete:
		if !there {
			return kind.Outcome[volumeKind.Status]{}, nil
		}

		err := daemon.RemoveVolume(ctx, found.Name, true)

		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return kind.Outcome[volumeKind.Status]{}, nil

		// one docker would not remove, which a container mounts, is left as
		// it was: removed later, once nobody remembered it was asked, it
		// would take what is in it with it.
		case errors.Is(err, docker.ErrInvalid):
			return kind.Outcome[volumeKind.Status]{Status: volumeKind.Status{
				Status:  kind.Status{State: volumeKind.Present, Reason: volumeKind.ReasonOf(found.Labels)},
				Docker:  volumeKind.DockerOf(found),
				Failure: blocks.Failure(err),
			}}, blocks.Refused(err)
		}

		return failed(err)
	}

	return failed(fmt.Errorf("%w: a volume cannot be %s on its node", kind.ErrUnknownAction, action))
}

// create makes a volume. One made before, which its status says it was, was
// made again after it went missing, and is labelled with when: what was in it
// is gone. A volume of its name that is not it is not taken for it, since
// docker would hand that one back as if it were made.
func (n *Node) create(ctx context.Context, daemon docker.Daemon, held []docker.Volume, v volumeKind.Volume) (docker.Volume, error) {
	for _, other := range held {
		if other.Name == v.Spec.Name {
			return docker.Volume{}, fmt.Errorf("%w: a volume named %s is there already, and it is not this one", docker.ErrInvalid, v.Spec.Name)
		}
	}

	labels := map[string]string{}
	if v.Status.Docker != nil {
		labels[volumeKind.LabelRecreated] = n.now().UTC().Format(time.RFC3339)
	}

	return daemon.CreateVolume(ctx, v.Spec.Docker(v.Metadata.UUID, labels))
}

// Query answers none of a volume's queries but its state, which is read off
// State.
func (n *Node) Query(_ context.Context, _ volumeKind.Volume, action string, _ any) (any, error) {
	return nil, fmt.Errorf("%w: a volume has no %q", kind.ErrUnknownAction, action)
}

// State is every volume of every running Docker VM on the node.
func (n *Node) State(ctx context.Context) (kind.Report[volumeKind.Status], error) {
	read, err := n.reader.Read(ctx)
	if err != nil {
		return kind.Report[volumeKind.Status]{}, err
	}

	report := blocks.Report[volumeKind.Status](read)

	for _, vmUUID := range read.VMs() {
		inventory := read.Inventories[vmUUID]
		stacks := blocks.Stacks(inventory)

		for _, held := range inventory.Volumes {
			report.Instances = append(report.Instances, kind.Observed[volumeKind.Status]{
				UUID:   blockKinds.Identity(volumeKind.Name, held.Labels),
				Owners: blocks.Owners(vmUUID, held.Labels, stacks),
				Status: volumeKind.Status{
					Status: kind.Status{State: volumeKind.Present, Reason: volumeKind.ReasonOf(held.Labels)},
					Docker: volumeKind.DockerOf(held),
				},
			})
		}
	}

	slices.SortFunc(report.Instances, func(a, b kind.Observed[volumeKind.Status]) int {
		return cmp.Or(strings.Compare(a.UUID, b.UUID), strings.Compare(a.Status.Docker.Name, b.Status.Docker.Name))
	})

	return report, nil
}

// find is the volume a resource is among what its VM's dockerd holds: the one
// labelled as the resource, or, for one nobody keeps a record of, the one its
// name names.
func find(held []docker.Volume, v volumeKind.Volume) (docker.Volume, bool) {
	for _, h := range held {
		if blockKinds.Identity(volumeKind.Name, h.Labels) == v.Metadata.UUID {
			return h, true
		}
	}

	if seen := v.Status.Docker; seen != nil && len(seen.Name) > 0 && len(blockKinds.Identity(volumeKind.Name, seen.Labels)) == 0 {
		for _, h := range held {
			if h.Name == seen.Name {
				return h, true
			}
		}
	}

	return docker.Volume{}, false
}

// failed is what a command that failed left a volume as: failed, saying why
// in the codes every side knows.
func failed(err error) (kind.Outcome[volumeKind.Status], error) {
	return kind.Outcome[volumeKind.Status]{Status: volumeKind.Status{
		Status:  kind.Status{State: volumeKind.Failed, Reason: err.Error()},
		Failure: blocks.Failure(err),
	}}, err
}
