// Package dependents is what deleting a resource the graphs of roots need does
// to those roots: which of them need it, and whether each makes it again
// (Of); and, when it is deleted with them (cascade), each marked to go first,
// to be removed by its graph (Mark), and waited on to go when it does not
// live in what is deleted, whose delete would otherwise be refused while they
// use it (Cascade). One that lives in it goes with it, as what lives in a VM
// goes with the VM.
//
// A root is marked to go as its own delete marks it, expected deleted and
// saying when it was first asked to go (dispatch.Dispatcher.Desire), but for
// what its kind would refuse its own delete for: a container that runs is
// refused its delete but by force, and its graph removes it by force all the
// same. What it is removed for is what it needs going, which no state of its
// own holds back.
package dependents

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// DefaultWait is how long a delete with its dependents waits for those
	// that do not live in what is deleted to go, before what is deleted is
	// asked for its delete, when whoever asked waits for nothing: a root's
	// graph removes a container at once, and by force, and a minute is long
	// enough for its node to, short of its VM being stopped, which nothing
	// waits out.
	DefaultWait = time.Minute

	// tries is how many times a root is read and marked again when something
	// else wrote it in the meantime.
	tries = 3
)

// Dependents are the roots whose graphs need what is deleted.
type Dependents struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	dispatcher *dispatch.Dispatcher

	// roots are the kinds whose roots their graphs make and remove, and
	// graphs what says which of them need a record, and what a root marked
	// to go is queued to.
	roots  graph.Roots
	graphs graph.Graphs
}

// New is what finds, marks and waits on the roots of the kinds roots names
// that need a record, as graphs says, over resources and dispatcher. With no
// graphs, or no kinds of root, nothing needs anything.
func New(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher, roots graph.Roots, graphs graph.Graphs) *Dependents {
	return &Dependents{registry: registry, resources: resources, dispatcher: dispatcher, roots: roots, graphs: graphs}
}

// Of are the roots whose graphs need r, a record of the kind descriptor
// describes, that were not asked to go, as the graphs say
// (graph.Dependents): none for a root, which no other root needs, and none
// where the graphs of roots are not kept.
func (d *Dependents) Of(ctx context.Context, descriptor kind.Descriptor, r resource.Record) ([]graph.Dependent, error) {
	if d == nil || d.graphs == nil || len(d.roots) == 0 || d.roots.Root(descriptor, r.Metadata) {
		return nil, nil
	}

	return d.graphs.Dependents(ctx, graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID})
}

// Cascade deletes the roots of dependents with r, which they need, before r
// is asked for its delete: each is marked to go, as a delete by force would,
// and queued to its graph, which removes it, and what it owns (Mark); and
// those that do not live in r are waited on to go, for as long as wait, or
// DefaultWait when wait is nothing, since r's node refuses r's delete while
// they use it. One that lives in r goes with it, and is not waited on. It is
// how long r's own delete is waited on after that: what is left of wait, and
// nothing when nothing was asked to be waited for. What marking one fails
// with is an error, and the rest are left as they were.
func (d *Dependents) Cascade(ctx context.Context, r resource.Record, dependents []graph.Dependent, wait time.Duration) (time.Duration, error) {
	if err := d.Mark(ctx, dependents); err != nil {
		return 0, err
	}

	patience := wait
	if patience <= 0 {
		patience = DefaultWait
	}

	deadline := time.Now().Add(patience)

	d.await(ctx, r, dependents, deadline)

	if wait <= 0 {
		return 0, nil
	}

	return max(time.Until(deadline), 0), nil
}

// Mark marks each root of dependents to go, as a delete by force would, and
// queues it to its graph, which removes it, and what it owns: one marked
// already is queued again, and one that is gone is gone already. What
// reading or writing one fails with is an error, and the rest are left as
// they were.
func (d *Dependents) Mark(ctx context.Context, dependents []graph.Dependent) error {
	for _, dependent := range dependents {
		if err := d.mark(ctx, dependent.Root); err != nil {
			return err
		}
	}

	return nil
}

// mark marks root to go, reading it again when something else wrote it
// first, and queues it.
func (d *Dependents) mark(ctx context.Context, root graph.Ref) error {
	for try := 1; ; try++ {
		r, err := d.resources.GetOne(ctx, root.Kind, root.UUID)
		switch {
		case errors.Is(err, domain.ErrNotExists):
			return nil
		case err != nil:
			return err
		}

		common, err := r.Common()
		if err != nil {
			return err
		}

		if common.Expected != kind.Deleted || r.Metadata.DeletionRequestedAt.IsZero() {
			_, err = d.dispatcher.Desire(ctx, r, kind.Deleted, true)
		}

		switch {
		case errors.Is(err, resource.ErrConflict) && try < tries:
			continue
		case errors.Is(err, domain.ErrNotExists):
			return nil
		case err != nil:
			return err
		}

		d.graphs.Enqueue(root)

		return nil
	}
}

// await waits for every root of dependents that does not live in r to go,
// until deadline: one that lives in r goes with it, and is not waited on.
func (d *Dependents) await(ctx context.Context, r resource.Record, dependents []graph.Dependent, deadline time.Time) {
	in := kind.Reference{Kind: r.Kind, UUID: r.Metadata.UUID}

	for _, dependent := range dependents {
		if !d.livesIn(ctx, dependent.Root, in) {
			d.dispatcher.Gone(ctx, dependent.Root.Kind, dependent.Root.UUID, time.Until(deadline))
		}
	}
}

// livesIn reports whether the record root names lives in the resource in
// names, as the kind it is of says what it lives in: it goes with it.
func (d *Dependents) livesIn(ctx context.Context, root graph.Ref, in kind.Reference) bool {
	binding, registered := d.registry.Lookup(root.Kind)
	if !registered || len(binding.Descriptor().Parent) == 0 || binding.Descriptor().Parent != in.Kind {
		return false
	}

	r, err := d.resources.GetOne(ctx, root.Kind, root.UUID)
	if err != nil {
		return false
	}

	parent, lives := r.Metadata.Owner(in.Kind)

	return lives && parent == in
}

// Recreated reports whether any root of dependents makes what they need again
// once it is gone: one that only names it, external to it, waits on it
// instead.
func Recreated(dependents []graph.Dependent) bool {
	for _, dependent := range dependents {
		if dependent.Recreates {
			return true
		}
	}

	return false
}
