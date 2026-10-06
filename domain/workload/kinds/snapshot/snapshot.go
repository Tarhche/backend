// Package snapshot is the snapshot kind: a VM's disk, kept as an archive,
// declared once for every service that runs it (domain/workload/kind).
//
// A snapshot's life is a series of operations rather than something a node
// keeps running, so what it is doing is known in the control plane, on its
// record, and nowhere else: no node reports it in its heartbeat, and its state
// is answered from the record. Of its operations, only taking it runs on a
// node: create is a command to the node holding the VM it is taken of, which
// streams the VM's disk into the snapshots bucket, under the VM's lock, and
// answers with how large the archive came out and which engine wrote it.
// Renaming it and deleting it, its archive with it, are the control plane's
// own.
//
// A snapshot is taken of a VM, its parent, and outlives it: deleting the VM,
// or restoring its disk, leaves its snapshots as they are. Restoring one is
// not the snapshot's action but the VM's (vm's restore, or a VM made with a
// snapshot as its source), which the snapshot kind says it can be restored
// onto, through Restores, rather than having the VM read its record.
//
// Its states are the ones the dashboard has always shown a snapshot in:
// creating, while it is taken; ready once its archive is stored, or failed
// when it could not be; and deleting, on its way out, and deleted once it is
// gone. A snapshot is never taken again: one that failed is deleted, and
// another is asked for.
package snapshot

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// Name is the kind's word, and Plural what its routes and its
	// permissions are named by: workload.snapshots.<verb>.
	Name   = "snapshot"
	Plural = "snapshots"

	// Parent is the kind a snapshot is taken of: a VM, which it outlives.
	Parent = "vm"
)

// A snapshot's states. Failed and Deleted are the framework's own.
const (
	// Creating is a snapshot its VM's node has been asked to take and store.
	Creating kind.State = "creating"

	// Ready is a snapshot whose archive is stored, and which can be restored.
	Ready kind.State = "ready"

	// Deleting is a snapshot whose archive is being taken away. Its record
	// goes with it.
	Deleting kind.State = "deleting"

	// Failed is a snapshot that could not be taken or stored. Its reason
	// says why.
	Failed = kind.Failed

	Deleted = kind.Deleted
)

// A snapshot's actions.
const (
	// ActionCreate takes a snapshot: the node holding its VM streams the VM's
	// disk into the snapshots bucket. It is the workload's own to ask for,
	// once the snapshot is admitted.
	ActionCreate = "create"

	// ActionRename gives a snapshot another name (RenamePayload), in the
	// control plane.
	ActionRename = "rename"

	// ActionDelete takes a snapshot's archive away from the bucket, in the
	// control plane, and its record with it.
	ActionDelete = "delete"

	// ActionState is what a snapshot is doing, which is its record.
	ActionState = "state"
)

// Limits of what a snapshot may be asked for, whoever asks.
const (
	// MaxNameLength keeps a name to something a listing can show.
	MaxNameLength = 100

	// TransferTimeout bounds streaming a VM's disk to the snapshots bucket or
	// from it: a snapshot taken, and a VM restored or made from one. A large
	// disk streamed to S3 takes minutes; one that has taken this long is not
	// going to finish, and the VM whose lock it holds cannot be stopped,
	// restored or deleted meanwhile. It is what kind.TimeoutTransfer is sized
	// as, on the nodes and in the control plane.
	TransferTimeout = 2 * time.Hour
)

// Spec is what a snapshot is asked for as: the VM it is taken of.
type Spec struct {
	VM VMRef `json:"vm"`
}

// VMRef is the VM a snapshot is taken of: its uuid, which is what it is
// asked for by, and what the VM was called when it was taken, which is what
// the snapshot is shown with once the VM is renamed or gone.
type VMRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name,omitempty"`
}

// Status is what a snapshot is doing, and what was taken.
type Status struct {
	kind.Status

	// Flavor and Image are those of the VM it was taken of: a VM it is
	// restored onto has to be of that flavor, and one made from it is of that
	// flavor and boots that image.
	Flavor vm.Kind `json:"flavor,omitempty"`
	Image  string  `json:"image,omitempty"`

	// Disk is the disk, in bytes, a VM it is restored onto needs at least.
	Disk uint64 `json:"disk,omitempty"`

	// Engine is what wrote the archive, as vm.Archive says it: only the same
	// engine can restore it.
	Engine string `json:"engine,omitempty"`

	// Size is how many bytes the archive takes where it is kept.
	Size int64 `json:"size,omitempty"`

	// CompletedAt is when its archive was stored.
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

// RenamePayload is the name a snapshot is given instead of its own.
type RenamePayload struct {
	Name string `json:"name"`
}

var _ domain.Validatable = &RenamePayload{}

func (p *RenamePayload) Validate() domain.ValidationErrors {
	if code, ok := ValidateName(p.Name); !ok {
		return domain.ValidationErrors{"name": code}
	}

	return nil
}

// ValidateName checks a snapshot's name: something, and something a listing
// can show.
func ValidateName(name string) (string, bool) {
	switch trimmed := strings.TrimSpace(name); {
	case len(trimmed) == 0:
		return "required_field", false
	case len(trimmed) > MaxNameLength:
		return "invalid_name", false
	}

	return "", true
}

// Snapshot is a snapshot, as its strategies are handed one.
type Snapshot = kind.Resource[Spec, Status]

// VMOf is the uuid of the VM a snapshot was taken of: its parent.
func VMOf(s Snapshot) string {
	if parent, ok := s.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return s.Spec.VM.UUID
}

// Descriptor is the snapshot kind, a new one every time.
func Descriptor() kind.Descriptor {
	return kind.Descriptor{
		Name:    Name,
		Plural:  Plural,
		StateBy: kind.OnControlPlane,
		Parent:  Parent,

		// a snapshot is an archive in a bucket, not something on its VM's
		// disk: deleting the VM, or restoring its disk, leaves it as it was.
		OnParent: kind.ParentRules{Delete: kind.CascadeKeep, Restore: kind.CascadeKeep},

		Machine: Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Creating}, Desires: Ready, Internal: true, Timeout: kind.TimeoutTransfer, Payload: kind.NoPayload},
			{Name: ActionRename, Runs: kind.OnControlPlane, Mode: kind.ModeCommand, AllowedIn: []kind.State{Ready, Failed}, Permission: "update", Payload: kind.Payload[RenamePayload]()},
			{Name: ActionDelete, Runs: kind.OnControlPlane, Mode: kind.ModeCommand, AllowedIn: []kind.State{Ready, Failed}, Desires: Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: ActionState, Runs: kind.OnControlPlane, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a snapshot's states and the moves between them.
//
// A snapshot is admitted creating, and is in flight until the answer to its
// create says how it went: ready, its archive stored, or failed. Nothing but
// that answer moves it, since no node reports a snapshot. Its delete runs in
// the control plane, and passes through deleting on its way to deleted.
//
// What is being taken is not deleted from under its node: a snapshot is
// deleted once it is ready or failed, so one asked to be deleted while it is
// being taken is expected deleted until its node has finished with it, and
// then deleted, archive and all. Nothing is left of it behind in the bucket.
func Machine() kind.Machine {
	return kind.Machine{
		Initial: Creating,
		States:  []kind.State{Creating, Ready, Failed, Deleting, Deleted},
		Transitions: []kind.Transition{
			{From: Creating, On: kind.OnObserved(Ready), To: Ready},
			{From: Creating, On: kind.OnObserved(Failed), To: Failed},
			{From: kind.Any, On: kind.OnAction(ActionDelete), To: Deleting},
			{From: Deleting, On: kind.OnObserved(Deleted), To: Deleted},
		},
		Terminal: []kind.State{Ready, Failed, Deleted},
		InFlight: []kind.State{Creating, Deleting},
	}
}

// ObjectKey is where a snapshot's archive is kept in the snapshots bucket.
func ObjectKey(uuid string) string {
	return "snapshots/" + uuid + ".msb"
}

// Store keeps the archives themselves, under their ObjectKey. An archive is
// streamed rather than held, so a size of -1 stores one whose length is not
// known until it ends. An archive that is not there is domain.ErrNotExists.
type Store interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, objectName string) error
}

// Why a snapshot cannot be restored onto a VM, in the codes the dashboard
// has words for.
const (
	// RefusedNotFound is a snapshot that is not there, or not the VM's
	// owner's.
	RefusedNotFound = "not_found"

	// RefusedNotReady is a snapshot whose archive is not stored: one still
	// being taken, or one that failed.
	RefusedNotReady = "snapshot_not_ready"

	// RefusedFlavor is a snapshot of a VM of another flavor.
	RefusedFlavor = "kind_mismatch"

	// RefusedDisk is a snapshot whose disk is larger than the VM's.
	RefusedDisk = "disk_too_small"

	// RefusedEngine is a snapshot written by an engine other than the one
	// the VM's node runs.
	RefusedEngine = "engine_mismatch"
)

// Target is what a snapshot is restored onto: one of its owner's VMs, or a
// VM made for its owner from it.
type Target struct {
	// OwnerUUID is whose the VM is. A snapshot is restored onto its owner's
	// VMs alone, and is not there for anybody else.
	OwnerUUID string

	// Flavor is the VM's, which the snapshot has to be of. None is a VM made
	// from it, which takes its flavor.
	Flavor vm.Kind

	// Disk is the VM's disk, in bytes, which the snapshot's has to fit in.
	// None is a VM made from it, whose disk grows to fit it.
	Disk uint64

	// Engine is the engine the VM's node runs, without its version, which
	// has to be the one that wrote the archive. None is a node that has not
	// said which, and holds the snapshot to nothing: whether an engine can
	// read an archive is the engine's to say, by failing the restore.
	Engine string
}

// Taken is what a snapshot took of its VM, which is what a VM restored from
// it, or made from it, is given.
type Taken struct {
	Flavor vm.Kind
	Image  string
	Disk   uint64
}

// Restores is the snapshot kind's own say on whether a snapshot can be
// restored onto a VM. The vm kind holds a restore, and a VM made from a
// snapshot, to it, rather than reading a snapshot's record itself.
type Restores interface {
	// Restorable is what the snapshot uuid names took of its VM, when it can
	// be restored onto onto; and otherwise why it cannot be, as one of the
	// Refused codes, with nothing taken. An error is only what kept it from
	// being looked at.
	Restorable(ctx context.Context, uuid string, onto Target) (Taken, string, error)
}

// Restorable is why s cannot be restored onto onto, as one of the Refused
// codes, or nothing when it can be: it has to be onto's owner's, ready, of
// onto's flavor, no larger than its disk, and written by the engine its node
// runs. What onto leaves out it is not held to.
func Restorable(s Snapshot, onto Target) string {
	switch {
	case s.Metadata.OwnerUUID != onto.OwnerUUID:
		return RefusedNotFound
	case s.Status.State != Ready || s.Status.Expected == Deleted:
		return RefusedNotReady
	case len(onto.Flavor) > 0 && s.Status.Flavor != onto.Flavor:
		return RefusedFlavor
	case onto.Disk > 0 && s.Status.Disk > onto.Disk:
		return RefusedDisk
	}

	if len(onto.Engine) == 0 || len(s.Status.Engine) == 0 {
		return ""
	}

	// which version of an engine can read it is the engine's to say.
	if written, _, _ := strings.Cut(s.Status.Engine, "/"); written != onto.Engine {
		return RefusedEngine
	}

	return ""
}
