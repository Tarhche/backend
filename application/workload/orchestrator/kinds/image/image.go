// Package image is the image kind's node strategy: what a node does to the
// images in its Docker VMs, and what it says they are.
//
// Nothing can label an image once it is pulled, so an image is known by the
// references its VM's dockerd holds it under: each of them is an image of the
// kind, under the uuid worked out from its VM and the reference
// (image.UUIDOf), as its record's was. An image nothing names any more is
// known by its Docker id.
package image

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
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
)

// Node is the image kind's node strategy.
type Node struct {
	reader *blocks.Reader

	// timeout bounds a command: waiting for its VM's dockerd, and a pull.
	timeout time.Duration
}

var _ kind.Node[imageKind.Spec, imageKind.Status] = &Node{}

// New is the strategy that reaches and reads Docker VMs through reader, each
// command given timeout.
func New(reader *blocks.Reader, timeout time.Duration) *Node {
	return &Node{reader: reader, timeout: timeout}
}

// Execute pulls an image into its VM, or removes it from there.
func (n *Node) Execute(ctx context.Context, i imageKind.Image, action string, _ any) (kind.Outcome[imageKind.Status], error) {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	daemon, err := n.reader.Reach(ctx, imageKind.VMOf(i))

	switch {
	case errors.Is(err, blocks.ErrGone) && action == imageKind.ActionDelete:
		return kind.Outcome[imageKind.Status]{}, nil
	case err != nil:
		return failed(err)
	}

	switch action {
	case imageKind.ActionCreate, imageKind.ActionPull:
		reference := imageKind.Normalized(i.Spec.Reference)

		pulled, err := daemon.PullImage(ctx, i.Spec.Reference)
		if err != nil {
			return failed(err)
		}

		return kind.Outcome[imageKind.Status]{Status: imageKind.Status{
			Status:  kind.Status{State: imageKind.Present},
			Docker:  imageKind.DockerOf(pulled, reference),
			Failure: blocks.Cleared(),
		}}, nil

	case imageKind.ActionDelete:
		// whether one a container uses may be removed was asked before it was
		// sent here: a delete that reached its node is to leave it gone.
		if err := daemon.RemoveImage(ctx, referenceOf(i), true); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return failed(err)
		}

		return kind.Outcome[imageKind.Status]{}, nil
	}

	return failed(fmt.Errorf("%w: an image cannot be %s on its node", kind.ErrUnknownAction, action))
}

// Query answers none of an image's queries but its state, which is read off
// State.
func (n *Node) Query(_ context.Context, _ imageKind.Image, action string, _ any) (any, error) {
	return nil, fmt.Errorf("%w: an image has no %q", kind.ErrUnknownAction, action)
}

// State is every image of every running Docker VM on the node, under each of
// the references it is held under, and one nothing names any more under its
// Docker id.
func (n *Node) State(ctx context.Context) (kind.Report[imageKind.Status], error) {
	read, err := n.reader.Read(ctx)
	if err != nil {
		return kind.Report[imageKind.Status]{}, err
	}

	report := blocks.Report[imageKind.Status](read)

	for _, vmUUID := range read.VMs() {
		for _, held := range read.Inventories[vmUUID].Images {
			report.Instances = append(report.Instances, Observed(vmUUID, held)...)
		}
	}

	slices.SortFunc(report.Instances, func(a, b kind.Observed[imageKind.Status]) int {
		return cmp.Compare(a.UUID, b.UUID)
	})

	return report, nil
}

// Observed is what is said of an image a Docker VM's dockerd listed: one
// image of the kind for each reference it is held under, and one under its
// Docker id when nothing names it any more.
func Observed(vmUUID string, held docker.Image) []kind.Observed[imageKind.Status] {
	owners := []kind.Reference{{Kind: imageKind.Parent, UUID: vmUUID}}

	references := imageKind.References(held)
	if len(references) == 0 {
		return []kind.Observed[imageKind.Status]{{
			UUID:   blockKinds.Derived(imageKind.Name, vmUUID, held.ID),
			Owners: owners,
			Status: imageKind.Status{Status: kind.Status{State: imageKind.Present}, Docker: imageKind.DockerOf(held, "")},
		}}
	}

	observed := make([]kind.Observed[imageKind.Status], 0, len(references))
	for _, reference := range references {
		observed = append(observed, kind.Observed[imageKind.Status]{
			UUID:   imageKind.UUIDOf(vmUUID, reference),
			Owners: owners,
			Status: imageKind.Status{Status: kind.Status{State: imageKind.Present}, Docker: imageKind.DockerOf(held, reference)},
		})
	}

	return observed
}

// referenceOf is what an image is removed as: the reference it was pulled
// as, or the one it was seen under, or its Docker id when nothing names it.
func referenceOf(i imageKind.Image) string {
	if len(strings.TrimSpace(i.Spec.Reference)) > 0 {
		return i.Spec.Reference
	}

	if seen := i.Status.Docker; seen != nil {
		return cmp.Or(seen.Reference, seen.ID)
	}

	return ""
}

// failed is what a command that failed left an image as: failed, saying why
// in the codes every side knows.
func failed(err error) (kind.Outcome[imageKind.Status], error) {
	return kind.Outcome[imageKind.Status]{Status: imageKind.Status{
		Status:  kind.Status{State: imageKind.Failed, Reason: err.Error()},
		Failure: blocks.Failure(err),
	}}, err
}
