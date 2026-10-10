package reconcile

import (
	"context"
	"maps"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Changed queues the roots the record ref names matters to, once it changed:
// it went missing, was deleted, was made or failed (graph.Notifier).
//
// What needs a record is what depends on it: what owns it, among its owners,
// and what uses it, which holds it among its refs, or, kept before refs, lives
// in it, which holds it among its owners. Each is looked for of every kind a
// graph is made of, through what belongs to the record (resource.Filter's
// Parent), which is what it lives in and what owns it alike, and is told
// apart by its refs. From ref up, what depends on each record reached is
// reached in turn, each once, up to the roots: a record of a kind of root
// that nothing owns, which is queued, and depends on nothing itself. A root
// that changed is queued itself, and so is a record of a kind of root that is
// gone, so that its lease, and how its plan went, are forgotten if it was
// one; and a record that is gone has no owners to say what owned it, and is
// found by what uses it alone, what owned it being found at the next resync.
//
// What a record of a kind that is no graph's needs, nothing depends on. With
// no kinds of root, nothing is read and nothing queued. What reading fails
// with is what it fails with, and the roots reached by then are queued.
func (r *Reconciler) Changed(ctx context.Context, ref graph.Ref) error {
	if len(r.roots) == 0 {
		return nil
	}

	reached := make(map[graph.Ref]bool)
	waiting := []graph.Ref{ref}

	for len(waiting) > 0 {
		next := waiting[0]
		waiting = waiting[1:]

		if reached[next] {
			continue
		}

		reached[next] = true

		dependents, err := r.dependents(ctx, next)
		if err != nil {
			return err
		}

		waiting = append(waiting, dependents...)
	}

	return nil
}

// dependents are what depends on the record ref names: what owns it, and
// what uses it. A root depends on nothing, and is queued instead.
func (r *Reconciler) dependents(ctx context.Context, ref graph.Ref) ([]graph.Ref, error) {
	if _, inGraph := r.kinds[ref.Kind]; !inGraph {
		return nil, nil
	}

	_, isRootKind := r.roots[ref.Kind]

	rec, there, err := r.read(ctx, ref)
	if err != nil {
		return nil, err
	}

	var dependents []graph.Ref

	switch owners := rec.Metadata.Owning(r.parentOf(ref.Kind)); {
	case there && isRootKind && len(owners) == 0:
		r.Enqueue(ref)

		return nil, nil
	case there:
		dependents = append(dependents, owners...)
	case isRootKind:
		r.Enqueue(ref)
	}

	for _, kindName := range slices.Sorted(maps.Keys(r.kinds)) {
		belonging, _, err := r.resources.GetAll(ctx, kindName, resource.Filter{Parent: ref}, 0, 0)
		if err != nil {
			return nil, err
		}

		for _, d := range belonging {
			if r.uses(d, ref) {
				dependents = append(dependents, graph.Ref{Kind: d.Kind, UUID: d.Metadata.UUID})
			}
		}
	}

	return dependents, nil
}

// uses reports whether d uses the record ref names: holds it among its refs,
// or, kept before refs, lives in it, which it holds among its owners. A
// record ref owns holds it among its owners too, and is what ref needs, not
// what needs it.
func (r *Reconciler) uses(d resource.Record, ref graph.Ref) bool {
	return slices.Contains(d.Metadata.Refs, ref) || (ref.Kind == r.parentOf(d.Kind) && slices.Contains(d.Metadata.Owners, ref))
}
