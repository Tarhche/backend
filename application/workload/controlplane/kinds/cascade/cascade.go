// Package cascade carries what happens to a resource over to the resources
// that live in it, by the rules their kinds declare (kind.ParentRules):
// deleting a Docker VM takes its stacks' records with it, and restoring its
// disk from a snapshot resets them to what the restored disk holds.
//
// It is told by whatever deletes or restores the parent, once that is done,
// and holds every kind registered to its rules alike: a kind added later
// whose resources live in VMs is carried along with no more than its rules.
package cascade

import (
	"context"
	"errors"

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

// Deleted is what deleting parent does to what lives in it, kind by kind:
// the records of a kind that goes with its parent are taken away, and those
// of a kind that outlives it are kept as they are.
func (c *Cascade) Deleted(ctx context.Context, parent kind.Reference) error {
	return c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Delete })
}

// Restored is what restoring parent from a snapshot does to what lives in
// it, kind by kind: the records of a kind that goes with its parent are taken
// away; those of a kind reset to what the parent holds are marked to be, so
// that the next look inside the parent keeps each as it is found, or takes
// its record away when it is not there; and those of a kind that outlives it
// are kept as they are.
func (c *Cascade) Restored(ctx context.Context, parent kind.Reference) error {
	return c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Restore })
}

// each applies to what lives in parent the rule rule says each kind keeps.
// One kind failing is no reason to leave the others as they were.
func (c *Cascade) each(ctx context.Context, parent kind.Reference, rule func(kind.Descriptor) kind.Cascade) error {
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
			failed = errors.Join(failed, c.children(ctx, d, parent, c.reset))
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

// reset marks a resource to be reset to what its parent holds, reading it
// again when something else wrote it first.
func (c *Cascade) reset(ctx context.Context, r resource.Record) error {
	for try := 1; ; try++ {
		if r.Reset {
			return nil
		}

		r.Reset = true

		_, err := c.resources.Update(ctx, r)
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
