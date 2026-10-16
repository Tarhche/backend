// Package snapshot is the snapshot kind's control-plane strategy: what a
// snapshot is admitted as, what it is asked for, what is done to it in the
// control plane, and whether it can be restored onto a VM.
//
// A snapshot is admitted of one of its owner's VMs that is running or
// stopped, on a node that can be asked, and only while its owner keeps fewer
// snapshots than one person may. It is placed where its VM is, belongs to its
// VM, and is admitted creating, with what it takes of the VM: its image,
// which says what the VM is, and its disk, and what the VM is called. Its
// create is asked for at
// once, of the node holding the VM, which takes it and answers with how large
// its archive came out and which engine wrote it. What is refused is said
// under the fields the dashboard asks with.
//
// It is admitted of one of the code runner's runs too, which is shown among
// anybody's VMs as the VM its snippet runs in (the vm kind's extras, Runs),
// while it runs: a run ends at its snippet's end, and goes at once. Its run
// is the guest's, so its snapshot is whoever took it, and counts among their
// snapshots, as one of a VM of theirs would; it outlives the run, as one of a
// VM outlives the VM, and is restored as a new VM of theirs, a machine booted
// from the runner's image with no snippet to run. Whoever asks is always whose
// a snapshot is: for a VM of theirs, the VM's owner.
//
// Its name is changed in the control plane (ActionRename), once it is ready
// or failed, and so is its delete, which takes its archive away from the
// snapshots bucket, an archive that is not there being one taken away
// already, and its record with it. One asked to be deleted while it is being
// taken is expected deleted, and deleted once its node has finished with it,
// so that nothing its node stores afterwards is left in the bucket.
//
// Whether a snapshot can be restored onto a VM is this kind's to say
// (snapshotKind.Restores), from its record: the vm kind asks before it
// restores a VM from one, or makes a VM from one. Whether the two are of one
// kind, a machine's onto a machine and a Docker VM's onto a Docker VM, their
// images say, read with the Docker image the control plane is given.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// VMs are the VMs snapshots are taken of.
type VMs interface {
	// GetOneByOwner is the VM uuid names, as one of ownerUUID's own:
	// somebody else's is domain.ErrNotExists.
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (vmKind.VM, error)
}

// Nodes say whether a node can be asked anything, having spoken lately.
type Nodes interface {
	Alive(ctx context.Context, nodeName string) (bool, error)
}

// Runs are the code runner's runs, which are shown among anybody's VMs, each
// as the VM its snippet runs in, and nobody keeps as a VM: the vm kind's
// extras (runs.Runs).
type Runs interface {
	// One is the run uuid names, as the VM it runs in, or
	// domain.ErrNotExists.
	One(ctx context.Context, uuid string) (kind.Raw, error)
}

// Dependencies are what the strategy reads and keeps snapshots in.
type Dependencies struct {
	VMs   VMs
	Nodes Nodes

	// Runs are the code runner's runs, which a snapshot is taken of as it is
	// of a VM, by whoever asks for one. None takes none.
	Runs Runs

	// Resources are where snapshots are kept, as every kind's resources are.
	Resources resource.Repository

	// Archives is the bucket an archive is taken away from when its
	// snapshot is deleted. None leaves archives where they are, and says so.
	Archives snapshotKind.Store

	// UserMax is the most snapshots one person may keep.
	UserMax uint

	// DockerImage is the image Docker VMs boot from, by which a snapshot's
	// image and a VM's say whether each is a Docker VM's (vm.KindOf).
	DockerImage string

	Logger *slog.Logger
}

// Snapshots is the snapshot kind's control-plane strategy.
type Snapshots struct {
	Dependencies
}

var (
	_ kind.ControlPlane[snapshotKind.Spec, snapshotKind.Status] = &Snapshots{}
	_ snapshotKind.Restores                                     = &Snapshots{}
)

func New(d Dependencies) *Snapshots {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}

	return &Snapshots{Dependencies: d}
}

// Admit takes in a snapshot its owner asked for, of one of their VMs that is
// running or stopped, on a node that can be asked: a disk is taken from a VM
// at rest or running, and one on its way somewhere is changing under it. A VM
// that is not there, or not theirs, is domain.ErrNotExists, unless it is one
// of the code runner's runs, which is anybody's to take a snapshot of while it
// runs: one that ran to its end goes at once, and has no disk to keep. The
// snapshot is whoever asked for it, of a run as of a VM of theirs, and is
// counted among theirs.
func (s *Snapshots) Admit(ctx context.Context, asked snapshotKind.Snapshot) (snapshotKind.Snapshot, domain.ValidationErrors, error) {
	vmUUID := snapshotKind.VMOf(asked)

	if invalid := validate(asked, vmUUID); len(invalid) > 0 {
		return snapshotKind.Snapshot{}, invalid, nil
	}

	v, run, err := s.vmOf(ctx, asked.Metadata.OwnerUUID, vmUUID)
	if err != nil {
		return snapshotKind.Snapshot{}, nil, err
	}

	alive, err := s.Nodes.Alive(ctx, v.Metadata.Node)
	if err != nil {
		return snapshotKind.Snapshot{}, nil, err
	}

	takeable := v.Status.State == vmKind.Running || (v.Status.State == vmKind.Stopped && !run)
	if !alive || !takeable {
		return snapshotKind.Snapshot{}, domain.ValidationErrors{"vm": "invalid_state_transition"}, nil
	}

	_, kept, err := s.Resources.GetAll(ctx, snapshotKind.Name, resource.Filter{OwnerUUID: asked.Metadata.OwnerUUID}, 0, 1)
	if err != nil {
		return snapshotKind.Snapshot{}, nil, err
	}

	if kept+1 > s.UserMax {
		return snapshotKind.Snapshot{}, domain.ValidationErrors{"snapshots": "quota_exceeded"}, nil
	}

	return snapshotKind.Snapshot{
		Kind: snapshotKind.Name,
		Metadata: kind.Metadata{
			Name:      strings.TrimSpace(asked.Metadata.Name),
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    maps.Clone(asked.Metadata.Labels),
			Owners:    slices.Clone(asked.Metadata.Owners),
			Refs:      kind.LivingIn(asked.Metadata.Refs, kind.Reference{Kind: snapshotKind.Parent, UUID: v.Metadata.UUID}),
			Node:      v.Metadata.Node,
		},
		Spec: snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: v.Metadata.UUID, Name: v.Metadata.Name}},
		Status: snapshotKind.Status{
			Status: kind.Status{State: snapshotKind.Creating, Expected: snapshotKind.Ready},
			Image:  v.Spec.Image,
			Disk:   v.Spec.Resources.Disk,
		},
	}, nil, nil
}

// vmOf is the VM a snapshot ownerUUID asked for is to be taken of, and
// whether it is one of the code runner's runs: one of ownerUUID's own VMs, or
// else the run uuid names, as the VM its snippet runs in, which is the
// guest's, and so nobody's own, and no VM's record either. Somebody has to be
// asking for a run's snapshot, since a snapshot is whoever took it. Neither
// being there is domain.ErrNotExists.
func (s *Snapshots) vmOf(ctx context.Context, ownerUUID string, uuid string) (vmKind.VM, bool, error) {
	v, err := s.VMs.GetOneByOwner(ctx, ownerUUID, uuid)
	if !errors.Is(err, domain.ErrNotExists) || s.Runs == nil || len(ownerUUID) == 0 {
		return v, false, err
	}

	run, err := s.Runs.One(ctx, uuid)
	if err != nil {
		return vmKind.VM{}, false, err
	}

	v, err = kind.Decode[vmKind.Spec, vmKind.Status](run)
	if err != nil {
		return vmKind.VM{}, false, err
	}

	return v, true, nil
}

// Reconcile asks for a snapshot admitted to be taken. A snapshot is never
// taken again: one that is ready or failed is asked for nothing, and one
// expected deleted is the reconcile loop's to delete.
func (s *Snapshots) Reconcile(_ context.Context, r snapshotKind.Snapshot) ([]kind.Intent, error) {
	if r.Status.State == snapshotKind.Creating && r.Status.Expected == snapshotKind.Ready {
		return []kind.Intent{{Action: snapshotKind.ActionCreate, Reason: "it was admitted, and is to be taken"}}, nil
	}

	return nil, nil
}

// Apply renames a snapshot, or deletes it: its archive is taken away from
// the bucket first, and its record goes once it is. An archive that cannot be
// taken away leaves the snapshot as it was, for its delete to be asked again.
func (s *Snapshots) Apply(ctx context.Context, r snapshotKind.Snapshot, action string, payload any) (snapshotKind.Snapshot, domain.ValidationErrors, error) {
	switch action {
	case snapshotKind.ActionRename:
		rename, _ := payload.(snapshotKind.RenamePayload)
		r.Metadata.Name = strings.TrimSpace(rename.Name)

		return r, nil, nil

	case snapshotKind.ActionDelete:
		if err := s.remove(ctx, r.Metadata.UUID); err != nil {
			return snapshotKind.Snapshot{}, nil, err
		}

		r.Status.State = snapshotKind.Deleted

		return r, nil, nil
	}

	return snapshotKind.Snapshot{}, nil, fmt.Errorf("%w: a snapshot has no %q run in the control plane", kind.ErrUnknownAction, action)
}

// remove takes a snapshot's archive away from the bucket. One that is not
// there is the outcome asked for: a snapshot that failed left nothing
// behind, and one deleted twice has nothing left the second time.
func (s *Snapshots) remove(ctx context.Context, uuid string) error {
	if s.Archives == nil {
		s.Logger.WarnContext(ctx, "no snapshots bucket is configured, so a snapshot's archive is left where it is", "snapshot", uuid)

		return nil
	}

	err := s.Archives.Delete(ctx, snapshotKind.ObjectKey(uuid))
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// Restorable is what the snapshot uuid names took of its VM, when it can be
// restored onto onto, or why it cannot be (snapshotKind.Restorable). A
// snapshot that is not onto's owner's is not there.
func (s *Snapshots) Restorable(ctx context.Context, uuid string, onto snapshotKind.Target) (snapshotKind.Taken, string, error) {
	if len(strings.TrimSpace(uuid)) == 0 || len(onto.OwnerUUID) == 0 {
		return snapshotKind.Taken{}, snapshotKind.RefusedNotFound, nil
	}

	r, err := s.Resources.GetOneByOwner(ctx, snapshotKind.Name, onto.OwnerUUID, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return snapshotKind.Taken{}, snapshotKind.RefusedNotFound, nil
	} else if err != nil {
		return snapshotKind.Taken{}, "", err
	}

	taken, err := kind.Decode[snapshotKind.Spec, snapshotKind.Status](r.Raw)
	if err != nil {
		return snapshotKind.Taken{}, "", err
	}

	if refused := snapshotKind.Restorable(taken, onto, s.DockerImage); len(refused) > 0 {
		return snapshotKind.Taken{}, refused, nil
	}

	return snapshotKind.Taken{Image: taken.Status.Image, Disk: taken.Status.Disk}, "", nil
}

// validate is what is wrong with a snapshot as it was asked for, that the
// request alone tells.
func validate(asked snapshotKind.Snapshot, vmUUID string) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	if code, ok := snapshotKind.ValidateName(asked.Metadata.Name); !ok {
		invalid["name"] = code
	}

	if len(strings.TrimSpace(vmUUID)) == 0 {
		invalid["vm_uuid"] = "required_field"
	}

	return invalid
}
