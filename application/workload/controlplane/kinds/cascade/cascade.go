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
//
// Where the roots of graphs are kept made (WithGraphs), a delete keeps what
// lives in the parent that a graph makes again: a root that was not asked to
// go, and what something still there, and not asked to go, owns. Each is kept
// as gone with its parent, missing and on no node, keeping what it makes its
// parent again with as the parent's record had it as it went, its slug among
// it, when its kind keeps any (kind.Outliver); and the graphs are told of it,
// and of what went, once the parent's record is gone: the roots then make
// their parent again, under its uuid, as it was, and then what they need in
// it, in order.
package cascade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
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

	// roots are the kinds whose roots are kept made, and notifier what is
	// told of what a parent's delete kept and took away: nothing is kept
	// with no notifier, nor with no kind of root.
	roots    graph.Roots
	notifier graph.Notifier

	now func() time.Time
}

// Option changes how a cascade goes about it.
type Option func(*Cascade)

// WithGraphs has a parent's delete keep what lives in it that the graphs of
// roots make again: a root of a kind roots names that was not asked to go,
// and what something still there, and not asked to go, owns, kept missing and
// on no node, as gone with its parent. notifier is told of each, and of what went, once the parent's
// record is gone, so that the roots whose graphs it is in make it again.
func WithGraphs(roots graph.Roots, notifier graph.Notifier) Option {
	return func(c *Cascade) {
		c.roots = roots
		c.notifier = notifier
	}
}

// WithClock has a cascade go by now, in place of the time now.
func WithClock(now func() time.Time) Option {
	return func(c *Cascade) {
		c.now = now
	}
}

func New(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, options ...Option) *Cascade {
	c := &Cascade{registry: registry, resources: resources, now: time.Now}

	for _, option := range options {
		option(c)
	}

	return c
}

// Repository keeps the resources of every kind in the repository it wraps,
// and deletes what lives in a resource before the resource itself, as each
// kind that lives in it says: deleting a VM's record takes its stacks' records
// with it, whichever part of the control plane deletes it. What lives in it
// goes first, so that a delete cut short leaves a VM with fewer stacks rather
// than stacks in a VM that is not there; what is kept of it is told of last,
// once the VM's record is gone, so that what makes it again finds the VM
// gone.
type Repository struct {
	resource.Repository

	cascade *Cascade
}

var _ resource.Repository = &Repository{}

// NewRepository wraps resources, carrying deletes over to what lives in what
// is deleted by the rules of the kinds in registry, as options say. What lives
// in what lives in it is carried along too, through the same repository.
func NewRepository(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, options ...Option) *Repository {
	r := &Repository{Repository: resources}
	r.cascade = New(registry, r, options...)

	return r
}

// Cascade is what the repository carries a parent's delete over with, which
// carries its restore over too.
func (r *Repository) Cascade() *Cascade {
	return r.cascade
}

// Delete takes away what lives in a resource, by its kind's rules, and then
// the resource, and then tells the graphs of roots what of it was kept or
// taken away. One that is not there is gone already, and so is what lived in
// it.
func (r *Repository) Delete(ctx context.Context, kindName string, uuid string) error {
	touched, err := r.cascade.deleted(ctx, kind.Reference{Kind: kindName, UUID: uuid})
	if err != nil {
		return err
	}

	if err := r.Repository.Delete(ctx, kindName, uuid); err != nil {
		return err
	}

	return r.cascade.changed(ctx, touched)
}

// Deleted is what deleting parent does to what lives in it, kind by kind:
// the records of a kind that goes with its parent are taken away, but for
// what the graphs of roots make again, when they are kept made, which is kept
// as gone with it; and those of a kind that outlives it are kept as they are.
// The graphs are told of what was kept or taken away once it is done.
func (c *Cascade) Deleted(ctx context.Context, parent kind.Reference) error {
	touched, err := c.deleted(ctx, parent)

	return errors.Join(err, c.changed(ctx, touched))
}

// Restored is what restoring parent from a snapshot, at a moment, does to
// what lives in it, kind by kind: the records of a kind that goes with its
// parent are taken away; those of a kind reset to what the parent holds are
// marked to be, as of that moment, so that the next look inside the parent
// taken since keeps each as it is found, or takes its record away when it is
// not there, and one taken before says nothing of them; and those of a kind
// that outlives it are kept as they are.
func (c *Cascade) Restored(ctx context.Context, parent kind.Reference, at time.Time) error {
	return c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Restore }, at, func(ctx context.Context, _ kind.ControlPlaneBinding, r resource.Record) error {
		return c.forget(ctx, r)
	})
}

// deleted is what deleting parent does to what lives in it (Deleted), but
// for telling the graphs, and what of it was kept or taken away, in the order
// it was. The parent's record is read first, as it is before it goes: what is
// kept of what lives in it keeps what makes it again from it.
func (c *Cascade) deleted(ctx context.Context, parent kind.Reference) ([]kind.Reference, error) {
	var touched []kind.Reference

	at := c.now()

	// a parent that is gone already has nothing of it to keep.
	var was kind.Raw
	if c.notifier != nil && len(c.roots) > 0 {
		if r, err := c.resources.GetOne(ctx, parent.Kind, parent.UUID); err == nil {
			was = r.Raw
		} else if !errors.Is(err, domain.ErrNotExists) {
			return nil, err
		}
	}

	failed := c.each(ctx, parent, func(d kind.Descriptor) kind.Cascade { return d.OnParent.Delete }, time.Time{}, func(ctx context.Context, b kind.ControlPlaneBinding, r resource.Record) error {
		if err := c.goes(ctx, b, r, was, at); err != nil {
			return err
		}

		touched = append(touched, kind.Reference{Kind: r.Kind, UUID: r.Metadata.UUID})

		return nil
	})

	return touched, failed
}

// each applies to what lives in parent the rule rule says each kind keeps,
// at a moment, what goes with it going as goes says. One kind failing is no
// reason to leave the others as they were.
func (c *Cascade) each(ctx context.Context, parent kind.Reference, rule func(kind.Descriptor) kind.Cascade, at time.Time, goes func(ctx context.Context, b kind.ControlPlaneBinding, r resource.Record) error) error {
	var failed error

	for _, binding := range c.registry.All() {
		d := binding.Descriptor()
		if d.Parent != parent.Kind {
			continue
		}

		switch rule(d) {
		case kind.CascadeDelete:
			failed = errors.Join(failed, c.children(ctx, d, parent, func(ctx context.Context, r resource.Record) error {
				return goes(ctx, binding, r)
			}))
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

// goes is what becomes of a resource of the kind b binds whose parent is
// deleted, at a moment: kept as gone with it, when a graph makes it again,
// keeping what it makes its parent again with as parent, the parent's record
// as it went, has it (kind.ControlPlaneBinding.Outlive); and taken away
// otherwise. One written in the meantime is read again, and judged again.
func (c *Cascade) goes(ctx context.Context, b kind.ControlPlaneBinding, r resource.Record, parent kind.Raw, at time.Time) error {
	d := b.Descriptor()

	for try := 1; ; try++ {
		kept, err := c.kept(ctx, d, r)
		switch {
		case err != nil:
			return err
		case !kept:
			return c.forget(ctx, r)
		}

		gone, err := missing(d, r, at)
		if err != nil {
			return err
		}

		if len(parent.Kind) > 0 {
			if gone.Raw, err = b.Outlive(gone.Raw, parent); err != nil {
				return err
			}
		}

		_, err = c.resources.Update(ctx, gone)
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

// kept reports whether a resource of d's kind whose parent is deleted is
// kept, which it is only where the roots of graphs are kept made, and some
// kind's resources are roots: a root that was not asked to go, and what
// something still there owns (kind.Metadata.Owning: what it lives in owns
// nothing), whose graph makes it again. What was asked to go never is, and
// an owner that was asked to go is not still there: what a root being
// removed owns goes, wherever the root is in its own removal.
func (c *Cascade) kept(ctx context.Context, d kind.Descriptor, r resource.Record) (bool, error) {
	if c.notifier == nil || len(c.roots) == 0 {
		return false, nil
	}

	asked, err := going(r)
	if err != nil || asked {
		return false, err
	}

	if c.roots.Root(d, r.Metadata) {
		return true, nil
	}

	for _, owner := range r.Metadata.Owning(d.Parent) {
		owning, err := c.resources.GetOne(ctx, owner.Kind, owner.UUID)
		switch {
		case errors.Is(err, domain.ErrNotExists):
			continue
		case err != nil:
			return false, err
		}

		ownerGoing, err := going(owning)
		switch {
		case err != nil:
			return false, err
		case !ownerGoing:
			return true, nil
		}
	}

	return false, nil
}

// going reports whether r was asked to go: expected deleted, deleted, or
// asked to be deleted once.
func going(r resource.Record) (bool, error) {
	common, err := r.Common()
	if err != nil {
		return false, err
	}

	return common.Expected == kind.Deleted || common.State == kind.Deleted || !r.Metadata.DeletionRequestedAt.IsZero(), nil
}

// missing is a resource of d's kind kept when its parent is deleted, as gone
// with it at a moment: in its kind's missing state, saying nothing of what its
// node last said of it, on no node, waiting on no command and tried for not at
// all, so that its graph makes it again where its parent is made again.
func missing(d kind.Descriptor, r resource.Record, at time.Time) (resource.Record, error) {
	common, err := r.Common()
	if err != nil {
		return resource.Record{}, err
	}

	status, err := resource.WithCommon(nil, kind.Status{
		State:      missingOf(d),
		Expected:   common.Expected,
		Reason:     fmt.Sprintf("it went with its %s", d.Parent),
		Since:      at,
		ObservedAt: at,
	})
	if err != nil {
		return resource.Record{}, err
	}

	r.Status = status
	r.Metadata.Node = ""
	r.Metadata.UpdatedAt = at
	r.Pending = nil
	r.Attempts = 0
	r.TriedAt = time.Time{}
	r.Reset = false

	return r, nil
}

// missingOf is the state a resource of d's kind is in once it is gone from
// where it lived: missing, when its kind has it, and the state it starts in,
// to be made, otherwise.
func missingOf(d kind.Descriptor) kind.State {
	if d.Machine.Has(kind.Missing) {
		return kind.Missing
	}

	return d.Machine.Initial
}

// changed tells the graphs of roots that each of refs changed, once what they
// lived in is gone, when they are kept made. What cannot be told is what it
// fails with, once every one is told.
func (c *Cascade) changed(ctx context.Context, refs []kind.Reference) error {
	if c.notifier == nil {
		return nil
	}

	var failed error

	for _, ref := range refs {
		if err := c.notifier.Changed(ctx, ref); err != nil {
			failed = errors.Join(failed, fmt.Errorf("the graphs %s/%s is in cannot be told it went with what it lived in: %w", ref.Kind, ref.UUID, err))
		}
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
