package execute

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Remove removes what plan made of root, the generation of it the plan is
// of, from the top down, through the edges that own what they need and no
// other, and is what that came to. Its root was asked to be deleted before,
// so that nothing makes anything of it again.
//
// What is removed is the plan's root and what it owns (graph.Plan.Owned):
// never what it uses loosely, or is only made after, nor anything external.
// Each is looked for first (Kind.Exists), as the record it is, made or not;
// one there is no record of is gone already, and is done with nothing to
// remove. One owned by a record that is not removed (graph.Ownership.Owners)
// is kept, and then so is what it owns, and what that owns, and so on, until
// nothing more is: a network two roots own outlives the delete of one. The
// root itself is always removed.
//
// Before anything is removed, every node kept is unlinked, a level of its own
// at once: what is removed is taken out of what owns it (Disown), so that
// nothing that stays is left owned by what is gone, and it is done, saying
// what keeps it. The rest is removed a level at a time, its own plan
// (graph.Plan.LevelsOf) from its last level to its first, ordered by every
// edge within it, a loose one too, so that nothing goes before what needs
// it: the root first, and, for a stack, its services in the reverse of the
// order they depend_on one another, and then their networks, and a volume
// it owns once the service that mounts it is gone. At its turn, each node
// but the root is removed (Kind.Delete) when nothing but what is removed owns
// it, as it is, so that while its removal waits it is still owned, and no
// root of its own; one that something came to own meanwhile is unlinked from
// what is removed, and kept instead, as, then, is what it owns.
//
// Every node is saved pending first, its action delete, as the record it is,
// and then each as it goes, those kept and those gone already among them, as
// Apply saves them: done once it is removed, unlinked or found gone, waiting
// on what its kind says it waits on (graph.Waiting), or failed. A level with
// a node that waits or failed is the last one started, and a run cancelled
// starts nothing more and saves what it cut short pending again, as Apply's
// do. Once every level is done, root's record is removed
// (graph.Ownership.Forget), last of all, when its kind's Delete left it, and
// then how its plan went (DeleteByRoot), since nothing of it is left to say
// it of.
//
// A plan with a node of a kind the executor is not given is carried out not
// at all, and is kind.ErrUnknownKind; nor is one with a record, or what owns
// one, that cannot be looked for, which is what looking for it failed with,
// naming its node. What fails to be saved, and what removing root's record,
// or how its plan went, fails with, is an error, returned beside how far the
// delete went.
func (e *Executor) Remove(ctx context.Context, root graph.Ref, generation int64, plan *graph.Plan) (Outcome, error) {
	if _, err := e.levelsOf(plan); err != nil {
		return Outcome{}, err
	}

	d, err := e.deletion(ctx, plan)
	if err != nil {
		return Outcome{}, err
	}

	levels := d.levels()

	r := e.newRun(plan)
	r.carry = d.carry

	now := e.now()

	err = r.start(ctx, levels, func(level int, node graph.Node) graph.Execution {
		return graph.Execution{
			Root:       root,
			Generation: generation,
			Node:       node.ID(),
			Kind:       node.Kind,
			Action:     graph.ActionDelete,
			Level:      level,
			State:      graph.StatePending,
			Ref:        d.records[node.ID()],
			UpdatedAt:  now,
		}
	})
	if err != nil {
		return r.outcome(levels), err
	}

	r.carryOut(ctx, levels)

	outcome := r.outcome(levels)
	if outcome.State != Done || r.err() != nil {
		return outcome, r.err()
	}

	if err := e.ownership.Forget(ctx, root); err != nil {
		return outcome, fmt.Errorf("the record of %s cannot be removed: %w", plan.Root(), err)
	}

	if err := e.executions.DeleteByRoot(context.WithoutCancel(ctx), root.UUID); err != nil {
		return outcome, fmt.Errorf("how the plan of %s went cannot be forgotten: %w", plan.Root(), err)
	}

	return outcome, nil
}

// deletion is what removing a root's plan does to the nodes of it, as their
// records were when it began.
type deletion struct {
	executor *Executor
	plan     *graph.Plan

	// candidates are the root and what it owns, the root first; records the
	// record of each there is one of, made or not; and kept those of them a
	// record that is not removed owns, by what owns each that is not.
	candidates []graph.NodeID
	records    map[graph.NodeID]graph.Ref
	kept       map[graph.NodeID][]graph.Ref

	// removed are the records of what is removed: the candidates' but those
	// kept, which one kept at its turn leaves.
	lock    sync.Mutex
	removed map[graph.Ref]bool
}

// deletion looks for the record of the plan's root, and of each node it
// owns, and for what owns each but the root, and keeps those a record that
// is not removed owns. What looking for either fails with is an error naming
// its node, and nothing is removed.
func (e *Executor) deletion(ctx context.Context, plan *graph.Plan) (*deletion, error) {
	root := plan.Root()

	d := &deletion{
		executor:   e,
		plan:       plan,
		candidates: append([]graph.NodeID{root}, plan.Owned(root)...),
		records:    make(map[graph.NodeID]graph.Ref),
		kept:       make(map[graph.NodeID][]graph.Ref),
		removed:    make(map[graph.Ref]bool),
	}

	// each one's record, made or not: one with none is gone already.
	for _, id := range d.candidates {
		node, _ := plan.Node(id)

		ref, _, err := e.kinds[node.Kind].Exists(ctx, node.Spec)
		if err != nil {
			return nil, fmt.Errorf("looking for %s: %w", id, err)
		}

		if ref != (graph.Ref{}) {
			d.records[id] = ref
			d.removed[ref] = true
		}
	}

	// what owns each, but the root, which is removed whatever owns it.
	owners := make(map[graph.NodeID][]graph.Ref)

	for _, id := range d.candidates[1:] {
		ref, recorded := d.records[id]
		if !recorded {
			continue
		}

		owning, err := e.ownership.Owners(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("looking for what owns %s: %w", id, err)
		}

		owners[id] = owning
	}

	// one owned by what is not removed is kept, and is not removed itself:
	// what it owns is then owned by what is not removed too, and so on,
	// until nothing more is kept. Each round is judged by what is removed as
	// it begins, so that what keeps a node is all that does.
	for {
		outside := make(map[graph.NodeID][]graph.Ref)

		for id, owning := range owners {
			if _, kept := d.kept[id]; kept {
				continue
			}

			for _, owner := range owning {
				if !d.removed[owner] {
					outside[id] = append(outside[id], owner)
				}
			}
		}

		if len(outside) == 0 {
			return d, nil
		}

		for id, owning := range outside {
			d.kept[id] = owning
			delete(d.removed, d.records[id])
		}
	}
}

// levels are the levels removing goes in: first, a level of its own, every
// node kept, and then the plan of the rest, ordered by every edge within it,
// from its last level to its first.
func (d *deletion) levels() [][]graph.NodeID {
	var levels [][]graph.NodeID

	if len(d.kept) > 0 {
		levels = append(levels, slices.Sorted(maps.Keys(d.kept)))
	}

	removing := slices.DeleteFunc(slices.Clone(d.candidates), func(id graph.NodeID) bool {
		_, kept := d.kept[id]

		return kept
	})

	planned := d.plan.LevelsOf(removing)
	slices.Reverse(planned)

	return append(levels, planned...)
}

// carry is what removing does to a node at its turn. One there is no record
// of is gone already. One kept is unlinked from what is removed, and done,
// saying what keeps it. The root is removed. Any other is removed when
// nothing but what is removed owns it then, as it is, still owned by what is
// removed with it, so that while its removal waits, on its VM say, it is
// never taken for a root of its own, which it would be once nothing owned it;
// one that something else still owns is unlinked from what is removed, and
// kept instead, and is no longer removed, so that what it owns is kept too.
//
// What owns a node may go between it being read and the node being unlinked:
// two roots that own one network, deleted at once, each finding the other
// owns it. One unlinked from what is removed is never left owned by nothing,
// which nothing would ever remove: at its turn, it is removed then, as it
// would have been had nothing else owned it; kept from the start, before
// what in it needs it is removed, it is owned by the root alone again, whose
// next delete removes it at its turn, or, once the root is gone, a resync,
// which removes what nothing left owns.
func (d *deletion) carry(ctx context.Context, node graph.Node) (graph.Ref, string, error) {
	id := node.ID()

	ref, recorded := d.records[id]
	if !recorded {
		return graph.Ref{}, "it is gone already", nil
	}

	if owners, kept := d.kept[id]; kept {
		remaining, err := d.executor.ownership.Disown(ctx, ref, d.removing(ref)...)
		if err != nil {
			return ref, "", err
		}

		if len(remaining) == 0 {
			return d.orphaned(ctx, ref)
		}

		return ref, keptBy(owners), nil
	}

	if id != d.plan.Root() {
		owners, err := d.executor.ownership.Owners(ctx, ref)
		if err != nil {
			return ref, "", err
		}

		if outside := d.outside(ref, owners); len(outside) > 0 {
			remaining, err := d.executor.ownership.Disown(ctx, ref, d.removing(ref)...)
			if err != nil {
				return ref, "", err
			}

			// what else owned it went meanwhile: it goes, nothing owning it.
			if len(remaining) > 0 {
				d.keep(ref)

				return ref, keptBy(remaining), nil
			}
		}
	}

	return ref, "", d.executor.kinds[node.Kind].Delete(ctx, ref)
}

// orphaned is ref, kept from the start, which, unlinked from what is removed,
// nothing owns any more, what else owned it having gone meanwhile: owned by
// the root alone again, it is kept for now, as what in it needs it is still
// there, and goes once the root does.
func (d *deletion) orphaned(ctx context.Context, ref graph.Ref) (graph.Ref, string, error) {
	root, recorded := d.records[d.plan.Root()]
	if !recorded {
		return ref, "", fmt.Errorf("nothing owns it any more, and %s, which it is kept for, is gone", d.plan.Root())
	}

	if err := d.executor.ownership.Link(ctx, ref, root, graph.Owned); err != nil {
		return ref, "", err
	}

	return ref, "kept, owned by " + root.Kind + "/" + root.UUID + " alone now: it goes once that is gone", nil
}

// outside are those of owners, what owns ref, that are not removed.
func (d *deletion) outside(ref graph.Ref, owners []graph.Ref) []graph.Ref {
	removing := d.removing(ref)

	var outside []graph.Ref
	for _, owner := range owners {
		if !slices.Contains(removing, owner) {
			outside = append(outside, owner)
		}
	}

	return outside
}

// removing are the records of what is removed but ref, in the order of their
// kinds and their uuids.
func (d *deletion) removing(ref graph.Ref) []graph.Ref {
	d.lock.Lock()
	defer d.lock.Unlock()

	var removing []graph.Ref
	for removed := range d.removed {
		if removed != ref {
			removing = append(removing, removed)
		}
	}

	return sorted(removing)
}

// keep takes ref out of what is removed.
func (d *deletion) keep(ref graph.Ref) {
	d.lock.Lock()
	defer d.lock.Unlock()

	delete(d.removed, ref)
}

// keptBy is why a node is kept: what owns it that is not removed.
func keptBy(owners []graph.Ref) string {
	said := make([]string, 0, len(owners))
	for _, owner := range sorted(owners) {
		said = append(said, owner.Kind+"/"+owner.UUID)
	}

	return "kept, owned by " + strings.Join(said, ", ")
}

// sorted are refs in the order of their kinds and their uuids, as a copy of
// their own.
func sorted(refs []graph.Ref) []graph.Ref {
	refs = slices.Clone(refs)
	slices.SortFunc(refs, func(a, b graph.Ref) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.UUID, b.UUID))
	})

	return refs
}
