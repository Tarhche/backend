// Package ownership is what owns the records a graph's nodes became, and
// what each uses, as the control plane keeps them (graph.Ownership): what
// owns a record is among its metadata's owners, and what it uses among its
// refs, the resource it lives in always among them.
//
// It reads and writes the control plane's records through its resource
// repository, and its kinds through its registry, which say what each kind
// lives in (kind.Descriptor.Parent). A resource of that kind never owns what
// lives in it: a VM requires nothing, so it owns nothing, and one among a
// record's owners is where a record kept before refs lives. That is what
// keeps such a record owned by what owns it alone.
//
// What it writes, it writes over the version it read, and reads again and
// writes again when something else wrote the record in the meantime, a
// heartbeat or the reconcile loop, three times at most. A record that is gone
// is nothing to read or write: owned by nothing, disowned, linked and
// forgotten already.
package ownership

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a record is read and written again when something
// else wrote it in the meantime.
const tries = 3

// Ownership is what owns the records of the kinds a registry holds, and what
// each uses, as a repository keeps them.
type Ownership struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	now       func() time.Time
}

var _ graph.Ownership = &Ownership{}

// New is what owns the records resources keeps of the kinds registry holds,
// and what each uses. Forget removes a root's record through resources, so it
// is the repository the control plane keeps its records in, which carries a
// delete over to what lives in what is deleted (cascade.Repository). A clock
// of nil is the time now.
func New(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, now func() time.Time) *Ownership {
	if now == nil {
		now = time.Now
	}

	return &Ownership{registry: registry, resources: resources, now: now}
}

// Owners are the records that own ref's record and are still there, in the
// order of their kinds and their uuids, each once: never one of the kind its
// kind lives in, which a record kept before refs has among its owners as
// where it lives. A record that is gone is owned by nothing. A record of a
// kind the registry does not hold is kind.ErrUnknownKind.
func (o *Ownership) Owners(ctx context.Context, ref graph.Ref) ([]graph.Ref, error) {
	r, there, err := o.read(ctx, ref)
	if err != nil || !there {
		return nil, err
	}

	return o.owning(ctx, r)
}

// Disown takes owners out of what owns ref's record, and is what owns it
// still, as Owners says it. One that owns it by none of them is not written.
// A record that is gone is disowned already, and owned by nothing.
func (o *Ownership) Disown(ctx context.Context, ref graph.Ref, owners ...graph.Ref) ([]graph.Ref, error) {
	r, there, err := o.change(ctx, ref, func(m *kind.Metadata) bool {
		kept := without(m.Owners, func(owner kind.Reference) bool { return slices.Contains(owners, owner) })
		if len(kept) == len(m.Owners) {
			return false
		}

		m.Owners = kept

		return true
	})
	if err != nil || !there {
		return nil, err
	}

	return o.owning(ctx, r)
}

// Link keeps the records of the two ends of an edge as its type says, and
// writes neither when each says so already:
//
//   - owned: dependent among what owns dependency, and dependency not among
//     what dependent uses, unless it is of the kind dependent lives in, which
//     stays among them whatever owns what;
//   - loose: dependency among what dependent uses, and dependent not among
//     what owns dependency;
//   - order: nothing.
//
// What a link gives is written before what it takes away, so that one cut
// short leaves both, as an owned edge and a loose one together keep it, and
// the next link takes up where it stopped. Either record that is gone is
// linked to nothing. An owned edge to a dependent of a kind the registry does
// not hold is kind.ErrUnknownKind, and a type that is none of them is an
// error.
func (o *Ownership) Link(ctx context.Context, dependency graph.Ref, dependent graph.Ref, t graph.EdgeType) error {
	switch t {
	case graph.Order:
		return nil

	case graph.Owned:
		d, err := o.descriptor(dependent.Kind)
		if err != nil {
			return err
		}

		if _, _, err := o.change(ctx, dependency, given(func(m *kind.Metadata) *[]kind.Reference { return &m.Owners }, dependent)); err != nil {
			return fmt.Errorf("%s/%s cannot be written as owned by %s/%s: %w", dependency.Kind, dependency.UUID, dependent.Kind, dependent.UUID, err)
		}

		// what it lives in it uses, whatever owns what.
		if dependency.Kind == d.Parent {
			return nil
		}

		if _, _, err := o.change(ctx, dependent, taken(func(m *kind.Metadata) *[]kind.Reference { return &m.Refs }, dependency)); err != nil {
			return fmt.Errorf("%s/%s cannot be written as not using %s/%s: %w", dependent.Kind, dependent.UUID, dependency.Kind, dependency.UUID, err)
		}

		return nil

	case graph.Loose:
		if _, _, err := o.change(ctx, dependent, given(func(m *kind.Metadata) *[]kind.Reference { return &m.Refs }, dependency)); err != nil {
			return fmt.Errorf("%s/%s cannot be written as using %s/%s: %w", dependent.Kind, dependent.UUID, dependency.Kind, dependency.UUID, err)
		}

		if _, _, err := o.change(ctx, dependency, taken(func(m *kind.Metadata) *[]kind.Reference { return &m.Owners }, dependent)); err != nil {
			return fmt.Errorf("%s/%s cannot be written as not owned by %s/%s: %w", dependency.Kind, dependency.UUID, dependent.Kind, dependent.UUID, err)
		}

		return nil
	}

	return fmt.Errorf("an edge is owned, loose or an order, and %d is none of them", t)
}

// Forget removes root's record, through the repository records are kept in,
// which carries the delete over to what lives in it, as the control plane's
// does. A root that is gone is forgotten already.
func (o *Ownership) Forget(ctx context.Context, root graph.Ref) error {
	return o.resources.Delete(ctx, root.Kind, root.UUID)
}

// owning are the records that own r and are still there, in the order of
// their kinds and their uuids, each once, but one of the kind r's kind lives
// in. What owns it that cannot be looked for is an error naming it.
func (o *Ownership) owning(ctx context.Context, r resource.Record) ([]graph.Ref, error) {
	d, err := o.descriptor(r.Kind)
	if err != nil {
		return nil, err
	}

	var owning []graph.Ref

	for _, owner := range r.Metadata.Owners {
		if owner.Kind == d.Parent || slices.Contains(owning, owner) {
			continue
		}

		_, there, err := o.read(ctx, owner)
		if err != nil {
			return nil, fmt.Errorf("looking for %s/%s, which owns %s/%s: %w", owner.Kind, owner.UUID, r.Kind, r.Metadata.UUID, err)
		}

		if there {
			owning = append(owning, owner)
		}
	}

	slices.SortFunc(owning, func(a, b graph.Ref) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.UUID, b.UUID))
	})

	return owning, nil
}

// change writes the record ref names as change leaves its metadata, over the
// version it read, reading it again and changing it again when something else
// wrote it in the meantime. One change leaves as it was is not written. It is
// the record as it was left, and whether there is one: one that is gone, or
// goes meanwhile, is not written.
func (o *Ownership) change(ctx context.Context, ref graph.Ref, change func(m *kind.Metadata) bool) (resource.Record, bool, error) {
	for try := 1; ; try++ {
		r, there, err := o.read(ctx, ref)
		if err != nil || !there {
			return resource.Record{}, false, err
		}

		if !change(&r.Metadata) {
			return r, true, nil
		}

		r.Metadata.UpdatedAt = o.now()

		written, err := o.resources.Update(ctx, r)
		switch {
		case err == nil:
			return written, true, nil
		case errors.Is(err, domain.ErrNotExists):
			return resource.Record{}, false, nil
		case !errors.Is(err, resource.ErrConflict) || try >= tries:
			return resource.Record{}, false, err
		}
	}
}

// read is the record ref names, and whether there is one.
func (o *Ownership) read(ctx context.Context, ref graph.Ref) (resource.Record, bool, error) {
	r, err := o.resources.GetOne(ctx, ref.Kind, ref.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return resource.Record{}, false, nil
	case err != nil:
		return resource.Record{}, false, err
	}

	return r, true, nil
}

// descriptor is the named kind's, as the registry holds it.
func (o *Ownership) descriptor(kindName string) (kind.Descriptor, error) {
	binding, registered := o.registry.Lookup(kindName)
	if !registered {
		return kind.Descriptor{}, fmt.Errorf("%w: %q", kind.ErrUnknownKind, kindName)
	}

	return binding.Descriptor(), nil
}

// given is a change that has ref among what of a record's metadata field
// says, once.
func given(field func(m *kind.Metadata) *[]kind.Reference, ref graph.Ref) func(m *kind.Metadata) bool {
	return func(m *kind.Metadata) bool {
		refs := field(m)
		if slices.Contains(*refs, ref) {
			return false
		}

		*refs = append(slices.Clone(*refs), ref)

		return true
	}
}

// taken is a change that has ref not among what of a record's metadata field
// says.
func taken(field func(m *kind.Metadata) *[]kind.Reference, ref graph.Ref) func(m *kind.Metadata) bool {
	return func(m *kind.Metadata) bool {
		refs := field(m)
		if !slices.Contains(*refs, ref) {
			return false
		}

		*refs = without(*refs, func(other kind.Reference) bool { return other == ref })

		return true
	}
}

// without are refs but those drop says, as a copy of their own, or nothing
// when none is left.
func without(refs []kind.Reference, drop func(kind.Reference) bool) []kind.Reference {
	kept := slices.DeleteFunc(slices.Clone(refs), drop)
	if len(kept) == 0 {
		return nil
	}

	return kept
}
