package reconcile

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
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
	roots, err := r.walk(ctx, ref)

	for _, root := range roots {
		r.Enqueue(root)
	}

	return err
}

// Dependents are the roots whose graphs need the record ref names
// (graph.Dependents): of the roots Changed would queue, those there, which
// nothing owns and which were not asked to go, whose plans have the record
// among their nodes, each saying whether its graph makes the record again
// or only names it, external to it, and waits on it while it is missing.
// Which node of a root's plan the record is, is what the kind of each node
// of its kind says it is (graph.Kind.Exists). A root is not its own
// dependent, and one whose plan has no node the record is depends on it no
// longer, whatever its record still says it uses.
//
// Nothing is queued, and nothing written. With no kinds of root, nothing is
// read, and no root needs anything. What reading the records, or planning or
// looking at a root's graph, fails with is what it fails with.
func (r *Reconciler) Dependents(ctx context.Context, ref graph.Ref) ([]graph.Dependent, error) {
	roots, err := r.walk(ctx, ref)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(roots, func(a, b graph.Ref) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.UUID, b.UUID))
	})

	var dependents []graph.Dependent

	for _, root := range slices.Compact(roots) {
		if root == ref {
			continue
		}

		dependent, needs, err := r.dependent(ctx, root, ref)
		if err != nil {
			return nil, err
		}

		if needs {
			dependents = append(dependents, dependent)
		}
	}

	return dependents, nil
}

// dependent is the root root as a dependent of the record ref names, and
// whether it is one: there, nothing owning it, not asked to go, and with a
// node of its plan that is the record.
func (r *Reconciler) dependent(ctx context.Context, root graph.Ref, ref graph.Ref) (graph.Dependent, bool, error) {
	rooted, isRoot := r.roots[root.Kind]
	if !isRoot {
		return graph.Dependent{}, false, nil
	}

	rec, there, err := r.read(ctx, root)
	switch {
	case err != nil:
		return graph.Dependent{}, false, err
	case !there, !r.unowned(rec), going(rec):
		return graph.Dependent{}, false, nil
	}

	requirement, err := rooted.Root(rec.Raw)
	if err != nil {
		return graph.Dependent{}, false, fmt.Errorf("what the root %s/%s needs cannot be read off its record: %w", root.Kind, root.UUID, err)
	}

	plan, err := r.planner.Plan(requirement)
	if err != nil {
		return graph.Dependent{}, false, fmt.Errorf("the graph of the root %s/%s cannot be planned: %w", root.Kind, root.UUID, err)
	}

	for _, node := range plan.Nodes() {
		if node.Kind != ref.Kind || node.ID() == plan.Root() {
			continue
		}

		k, known := r.kinds[node.Kind]
		if !known {
			continue
		}

		found, _, err := k.Exists(ctx, node.Spec)
		if err != nil {
			return graph.Dependent{}, false, fmt.Errorf("whether %s of the root %s/%s is there: %w", node.ID(), root.Kind, root.UUID, err)
		}

		if found == ref {
			return graph.Dependent{Root: root, Name: rec.Metadata.Name, Recreates: !node.External}, true, nil
		}
	}

	return graph.Dependent{}, false, nil
}

// walk is every root the record ref names matters to, as Changed reaches
// them, each once, in the order they are reached: the roots, and the records
// of a kind of root that are gone. What reading fails with is what it fails
// with, beside the roots reached by then.
func (r *Reconciler) walk(ctx context.Context, ref graph.Ref) ([]graph.Ref, error) {
	if len(r.roots) == 0 {
		return nil, nil
	}

	var roots []graph.Ref

	reached := make(map[graph.Ref]bool)
	waiting := []graph.Ref{ref}

	for len(waiting) > 0 {
		next := waiting[0]
		waiting = waiting[1:]

		if reached[next] {
			continue
		}

		reached[next] = true

		dependents, rooted, err := r.dependents(ctx, next)
		if rooted {
			roots = append(roots, next)
		}

		if err != nil {
			return roots, err
		}

		waiting = append(waiting, dependents...)
	}

	return roots, nil
}

// dependents are what depends on the record ref names, what owns it and what
// uses it, and whether it is a root: one there that nothing owns, or one of
// a kind of root that is gone, which depends on nothing reached through what
// owns it.
func (r *Reconciler) dependents(ctx context.Context, ref graph.Ref) ([]graph.Ref, bool, error) {
	if _, inGraph := r.kinds[ref.Kind]; !inGraph {
		return nil, false, nil
	}

	_, isRootKind := r.roots[ref.Kind]

	rec, there, err := r.read(ctx, ref)
	if err != nil {
		return nil, false, err
	}

	var (
		dependents []graph.Ref
		rooted     bool
	)

	switch owners := rec.Metadata.Owning(r.parentOf(ref.Kind)); {
	case there && isRootKind && len(owners) == 0:
		return nil, true, nil
	case there:
		dependents = append(dependents, owners...)
	case isRootKind:
		rooted = true
	}

	for _, kindName := range slices.Sorted(maps.Keys(r.kinds)) {
		belonging, _, err := r.resources.GetAll(ctx, kindName, resource.Filter{Parent: ref}, 0, 0)
		if err != nil {
			return nil, rooted, err
		}

		for _, d := range belonging {
			if r.uses(d, ref) {
				dependents = append(dependents, graph.Ref{Kind: d.Kind, UUID: d.Metadata.UUID})
			}
		}
	}

	return dependents, rooted, nil
}

// uses reports whether d uses the record ref names: holds it among its refs,
// or, kept before refs, lives in it, which it holds among its owners. A
// record ref owns holds it among its owners too, and is what ref needs, not
// what needs it.
func (r *Reconciler) uses(d resource.Record, ref graph.Ref) bool {
	return slices.Contains(d.Metadata.Refs, ref) || (ref.Kind == r.parentOf(d.Kind) && slices.Contains(d.Metadata.Owners, ref))
}

// going reports whether rec was asked to go: expected deleted, or asked to
// be deleted once.
func going(rec resource.Record) bool {
	common, err := rec.Common()

	return !rec.Metadata.DeletionRequestedAt.IsZero() || (err == nil && common.Expected == kind.Deleted)
}
