// Package resource is a resource of any kind as the control plane keeps it:
// its manifest, and what the control plane is in the middle of asking of it.
//
// The manifest is the kind's own (kind.Raw): its metadata, its spec and its
// status, as every service and the API see it. Beside it, the control plane
// keeps what is its own business and nobody else's: the command a resource in
// flight is waiting on, how many tries it has taken to make the resource what
// it is expected to be, what the last command came to, and the version that
// keeps two writers from overwriting each other blindly.
package resource

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// ErrConflict is a copy of a resource written back after something else wrote
// the resource since the copy was read: a heartbeat and a command's result, a
// person and the reconcile loop. Whoever is refused reads the resource again
// and decides again, on what is there now.
var ErrConflict = errors.New("the resource was changed after this copy of it was read")

// Record is one resource as the control plane keeps it.
type Record struct {
	kind.Raw

	// Version is how many times the record was written. A copy is written
	// back only over the version it was read at, which the repository checks
	// and moves on: that is what holds a heartbeat and a result, or a person
	// and the reconcile loop, from writing over each other.
	Version int64

	// Pending is the command the resource was last sent and has not been
	// answered for, while it is waiting on one.
	Pending *Pending

	// Attempts is how many commands were sent to make the resource what it is
	// expected to be, since it last stayed so for as long as its backoff, or
	// since it was last asked for something new. It is what the next try's
	// Attempt is, and what the wait before it grows with: one that keeps
	// falling over as soon as it is brought back is brought back less and
	// less often.
	Attempts int

	// TriedAt is when the last of those commands was sent.
	TriedAt time.Time

	// Answer is what came of the last command answered: whoever waited for
	// it and was not the one that heard it reads it here. Its status is not
	// kept, since it is the resource's already.
	Answer *kind.ResourceActedOn

	// Reset says the parent the resource lives in was restored from a
	// snapshot since its node last saw it there, and its kind resets to what
	// the restored parent holds (kind.CascadeReset): the next look inside the
	// parent keeps it as it is found, and takes its record away when it is
	// not there, rather than making it again.
	Reset bool
}

// Pending is a command a resource is waiting on.
//
// A command that is not answered in time is sent again, under a new ID, as
// another try at the same thing: a result for any of its tries answers it,
// while one for a command sent before it does not.
type Pending struct {
	Action  string
	Payload json.RawMessage

	// IDs are the commands sent for it, the first try first.
	IDs []string

	// SentAt is when the latest try was sent.
	SentAt time.Time
}

// Answers reports whether the command id is one of its tries.
func (p *Pending) Answers(id string) bool {
	return p != nil && len(id) > 0 && slices.Contains(p.IDs, id)
}

// Common is the part of the resource's status every kind shares: what it is
// doing, what it is expected to be doing, and why.
func (r Record) Common() (kind.Status, error) {
	return Common(r.Status)
}

// SetCommon writes the part of the resource's status every kind shares,
// leaving the kind's own fields as they are.
func (r *Record) SetCommon(common kind.Status) error {
	status, err := WithCommon(r.Status, common)
	if err != nil {
		return err
	}

	r.Status = status

	return nil
}

// Clone is a copy of the record that shares nothing with it.
func (r Record) Clone() Record {
	r.Metadata.Labels = maps.Clone(r.Metadata.Labels)
	r.Metadata.Owners = slices.Clone(r.Metadata.Owners)
	r.Spec = slices.Clone(r.Spec)
	r.Status = slices.Clone(r.Status)

	if r.Pending != nil {
		pending := *r.Pending
		pending.Payload = slices.Clone(pending.Payload)
		pending.IDs = slices.Clone(pending.IDs)
		r.Pending = &pending
	}

	if r.Answer != nil {
		answer := *r.Answer
		answer.Status = slices.Clone(answer.Status)
		r.Answer = &answer
	}

	return r
}

// Filter narrows a listing of one kind's resources. What is left empty does
// not narrow it.
type Filter struct {
	// OwnerUUID is the person they belong to.
	OwnerUUID string

	// Node is the node holding them.
	Node string

	// Parent is a resource they belong to: one of their metadata's owners.
	// Its kind may be left out, and then a resource of any kind with that
	// uuid is.
	Parent kind.Reference

	// Labels are labels they carry, each with the value given: the Docker
	// VMs are the VMs labelled workload.flavor=docker.
	Labels map[string]string
}

// Passes reports whether a resource's metadata is what filter lets through,
// for whoever narrows resources that are not in a repository, such as a
// kind's extras.
func (f Filter) Passes(m kind.Metadata) bool {
	if len(f.OwnerUUID) > 0 && m.OwnerUUID != f.OwnerUUID {
		return false
	}

	if len(f.Node) > 0 && m.Node != f.Node {
		return false
	}

	if len(f.Parent.UUID) > 0 && !slices.ContainsFunc(m.Owners, func(owner kind.Reference) bool {
		return owner.UUID == f.Parent.UUID && (len(f.Parent.Kind) == 0 || owner.Kind == f.Parent.Kind)
	}) {
		return false
	}

	for key, value := range f.Labels {
		if carried, ok := m.Labels[key]; !ok || carried != value {
			return false
		}
	}

	return true
}

// Repository keeps the resources of every kind, each kind apart from the
// others: one kind's resources are never another's, whatever their uuids.
//
// A listing is newest first.
type Repository interface {
	// Create keeps a new resource. One whose uuid or slug another resource of
	// its kind has already is domain.ErrAlreadyExists. It comes back as it is
	// kept: given a uuid when it had none, at its first version.
	Create(ctx context.Context, r Record) (Record, error)

	// Update writes a resource back over the version it was read at, and is
	// the resource at its next version. It is ErrConflict when something
	// wrote it since, domain.ErrNotExists when it is gone, and
	// domain.ErrAlreadyExists when its slug is another's.
	Update(ctx context.Context, r Record) (Record, error)

	// Delete takes a resource away. One that is not there is gone already.
	Delete(ctx context.Context, kindName string, uuid string) error

	// GetOne is the resource of the kind that uuid names, or
	// domain.ErrNotExists.
	GetOne(ctx context.Context, kindName string, uuid string) (Record, error)

	// GetOneByOwner is GetOne narrowed to one person's own: somebody else's
	// is not there as far as they are concerned.
	GetOneByOwner(ctx context.Context, kindName string, ownerUUID string, uuid string) (Record, error)

	// GetOneBySlug is the resource of the kind that slug names, or
	// domain.ErrNotExists.
	GetOneBySlug(ctx context.Context, kindName string, slug string) (Record, error)

	// GetAll is a page of the kind's resources that filter lets through,
	// limit of them from offset on, and how many it lets through in all. A
	// limit of zero is every one of them from offset on.
	GetAll(ctx context.Context, kindName string, filter Filter, offset uint, limit uint) ([]Record, uint, error)
}
