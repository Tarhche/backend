// Package cascade carries what happens to a resource over to the resources
// that live in it, by the rules their kinds declare (kind.ParentRules):
// deleting a Docker VM takes its stacks' records with it, and restoring its
// disk from a snapshot resets them to what the restored disk holds.
//
// A delete is carried over wherever a record is deleted from, by the
// Repository every part of the control plane keeps resources in: a VM's
// record goes when its node says it is gone, when it was deleted on a node
// that went quiet, or when it was on none, and what lives in it goes first,
// each time. A restore is told by whatever hears that it was carried out.
// Every kind registered is held to its rules alike: a kind added later whose
// resources live in VMs is carried along with no more than its rules.
package cascade

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a record is read and written again when something
// else wrote it in the meantime.
const tries = 3

// Cascade applies what happens to parents to what lives in them.
type Cascade struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
}

func New(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository) *Cascade {
	return &Cascade{registry: registry, resources: resources}
}

// Repository keeps the resources of every kind in the repository it wraps,
// and deletes what lives in a resource before the resource itself, as each
// kind that lives in it says: deleting a VM's record takes its stacks'
// records with it, whichever part of the control plane deletes it. What lives
// in it goes first, so that a delete cut short leaves a VM with fewer stacks
// rather than stacks in a VM that is not there.
type Repository struct {
	resource.Repository

	cascade *Cascade
}

var _ resource.Repository = &Repository{}

// NewRepository wraps resources, carrying deletes over to what lives in what
// is deleted by the rules of the kinds in registry. What lives in what lives
// in it is carried along too, through the same repository.
func NewRepository(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository) *Repository {
	r := &Repository{Repository: resources}
	r.cascade = New(registry, r)

	return r
}

// Cascade is what the repository carries a parent's delete over with, which
// carries its restore over too.
func (r *Repository) Cascade() *Cascade {
	return r.cascade
}

// Delete takes away what lives in a resource, by its kind's rules, and then
// the resource. One that is not there is gone already, and so is what lived
// in it.
func (r *Repository) Delete(ctx context.Context, kindName string, uuid string) error {
	if err := r.cascade.Deleted(ctx, kind.Reference{Kind: kindName, UUID: uuid}); err != nil {
		return err
	}

	return r.Repository.Delete(ctx, kindName, uuid)
}

// Deleted is what deleting parent does to what lives in it, kind by kind:
// the records of a kind that goes with its parent are taken away, and those
// of a kind that outlives it are kept as they are.
func (c *Cascade) Deleted(ctx context.Context, parent kind.Reference) error {
	return c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Delete }, time.Time{})
}

// Restored is what restoring parent from a snapshot, at a moment, does to
// what lives in it, kind by kind: the records of a kind that goes with its
// parent are taken away; those of a kind reset to what the parent holds are
// marked to be, as of that moment, so that the next look inside the parent
// taken since keeps each as it is found, or takes its record away when it is
// not there, and one taken before says nothing of them; and those of a kind
// that outlives it are kept as they are.
func (c *Cascade) Restored(ctx context.Context, parent kind.Reference, at time.Time) error {
	return c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Restore }, at)
}

// each applies to what lives in parent the rule rule says each kind keeps,
// at a moment. One kind failing is no reason to leave the others as they
// were.
func (c *Cascade) each(ctx context.Context, parent kind.Reference, rule func(kind.Descriptor) kind.Cascade, at time.Time) error {
	var failed error

	for _, binding := range c.registry.All() {
		d := binding.Descriptor()
		if d.Parent != parent.Kind {
			continue
		}

		switch rule(d) {
		case kind.CascadeDelete:
			failed = errors.Join(failed, c.children(ctx, d, parent, c.forget))
		case kind.CascadeReset:
			failed = errors.Join(failed, c.children(ctx, d, parent, func(ctx context.Context, r resource.Record) error {
				return c.reset(ctx, r, at)
			}))
		}
	}

	return failed
}

// children does what to every resource of d's kind that lives in parent.
func (c *Cascade) children(ctx context.Context, d kind.Descriptor, parent kind.Reference, what func(ctx context.Context, r resource.Record) error) error {
	records, _, err := c.resources.GetAll(ctx, d.Name, resource.Filter{Parent: parent}, 0, 0)
	if err != nil {
		return err
	}

	var failed error
	for i := range records {
		failed = errors.Join(failed, what(ctx, records[i]))
	}

	return failed
}

// forget takes a resource's record away.
func (c *Cascade) forget(ctx context.Context, r resource.Record) error {
	return c.resources.Delete(ctx, r.Kind, r.Metadata.UUID)
}

// reset marks a resource to be reset to what its parent holds as of a
// moment, reading it again when something else wrote it first: it is taken to
// have been observed then, so that what was seen of it before is older than
// what is known of it.
func (c *Cascade) reset(ctx context.Context, r resource.Record, at time.Time) error {
	for try := 1; ; try++ {
		common, err := r.Common()
		if err != nil {
			return err
		}

		if r.Reset && !common.ObservedAt.Before(at) {
			return nil
		}

		r.Reset = true

		if common.ObservedAt.Before(at) {
			common.ObservedAt = at

			if err := r.SetCommon(common); err != nil {
				return err
			}
		}

		_, err = c.resources.Update(ctx, r)
		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return nil
		case !errors.Is(err, resource.ErrConflict) || try >= tries:
			return err
		}

		r, err = c.resources.GetOne(ctx, r.Kind, r.Metadata.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return nil
		} else if err != nil {
			return err
		}
	}
}
