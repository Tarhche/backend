package reconcile

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// removeUnplanned removes root, asked to go, whose record, rec, says nothing
// a plan can be made of, as unplanned says: a stack's file kept before the
// platform read files as it does now, say. What a plan's delete would remove
// is found through the records alone, as a delete's cascade finds what lives
// in what it deletes: each record that has root among its owners, each that
// has one of those among its own, and so on. One that something that stays
// owns too is kept, as a plan's delete keeps it, and so then is what it owns.
// The rest is removed through its kind (graph.Kind.Delete), as it is, the
// root first, and each once nothing removed owns it or uses it: a container
// before the networks it joins, and a volume something removed owns after
// what mounts it. Then root's record goes, last of all. What root only uses,
// its VM and its volumes, it never owns, and they stay.
//
// It runs under root's lease (held), and is how the run ended, as carry
// says: gone once root's record is; waiting on what a kind's delete waits
// on, its VM say, and taken up by the next run where it stopped; cut short
// once ctx is done, its lease lost or its control plane stopping, whatever
// it was doing then, looking for the records, removing one of them or root's
// own, and removing nothing more, though what it was removing went all the
// same, a kind's delete that does not wait on ctx say; or failed with what a
// delete, or looking for the records, failed with.
func (r *Reconciler) removeUnplanned(ctx context.Context, root graph.Ref, rec resource.Record, unplanned error) (graph.Ended, bool, error) {
	r.logger.WarnContext(ctx, "a root asked to go whose plan cannot be made is removed through what its records say it owns", "error", unplanned, "kind", root.Kind, "uuid", root.UUID)

	removing, err := r.owning(ctx, root, rec)

	switch {
	case err != nil && ctx.Err() != nil:
		return r.cutShort(ctx, "looking for what "+root.Kind+"/"+root.UUID+" owns")
	case err != nil:
		return r.failed(fmt.Errorf("%w, and what it owns cannot be looked for: %w", unplanned, err))
	}

	for _, level := range removing.levels() {
		for _, ref := range level {
			// nothing more is removed once the run is cut short, as an
			// executor's run starts nothing more.
			if ctx.Err() != nil {
				return r.cutShort(ctx, "before "+ref.Kind+"/"+ref.UUID)
			}

			var waiting *graph.Waiting

			switch err := r.kinds[ref.Kind].Delete(ctx, ref); {
			case errors.As(err, &waiting):
				reason := ref.Kind + "/" + ref.UUID + ": " + waiting.Reason

				r.logger.DebugContext(ctx, "a root whose plan cannot be made waits to be removed", "kind", root.Kind, "uuid", root.UUID, "reason", reason)

				return graph.Ended{State: graph.StateWaiting, Reason: reason}, false, nil
			case err != nil && ctx.Err() != nil:
				return r.cutShort(ctx, ref.Kind+"/"+ref.UUID)
			case err != nil:
				return r.failed(fmt.Errorf("%s/%s cannot be removed: %w", ref.Kind, ref.UUID, err))
			}
		}
	}

	if ctx.Err() != nil {
		return r.cutShort(ctx, "before the record of "+root.Kind+"/"+root.UUID)
	}

	switch err := r.resources.Delete(ctx, root.Kind, root.UUID); {
	case err == nil, errors.Is(err, domain.ErrNotExists):
	case ctx.Err() != nil:
		return r.cutShort(ctx, "the record of "+root.Kind+"/"+root.UUID)
	default:
		return r.failed(fmt.Errorf("the record of %s/%s cannot be removed: %w", root.Kind, root.UUID, err))
	}

	r.logger.InfoContext(ctx, "removed a root asked to go whose plan could not be made, and what its records said it owned", "kind", root.Kind, "uuid", root.UUID, "removed", len(removing.records)-1)

	return graph.Ended{}, true, nil
}

// cutShort is a removal cut short once ctx is done, its lease lost or its
// control plane stopping, as it did what says, or was about to: no failure
// of it, saying what cut it short (context.Cause).
func (r *Reconciler) cutShort(ctx context.Context, what string) (graph.Ended, bool, error) {
	return r.failed(fmt.Errorf("%w: %s: %w", errCutShort, what, context.Cause(ctx)))
}

// owned is a root and what its records say it owns, which goes with it, each
// record by its ref.
type owned struct {
	reconciler *Reconciler
	root       graph.Ref
	records    map[graph.Ref]resource.Record
}

// owning is root, whose record is rec, and what the records say it owns, but
// what something that stays owns too: each record, of a kind of a graph, that
// has root among its owners, then each that has one of those among its own,
// and so on, until nothing more is found. One owned by a record that is there
// and not among them is kept, and is not among them itself: what it owns is
// then owned by what stays too, and so on, until nothing more is kept. What
// looking for the records fails with is what it fails with.
func (r *Reconciler) owning(ctx context.Context, root graph.Ref, rec resource.Record) (*owned, error) {
	o := &owned{reconciler: r, root: root, records: map[graph.Ref]resource.Record{root: rec}}

	for waiting := []graph.Ref{root}; len(waiting) > 0; waiting = waiting[1:] {
		owner := waiting[0]

		for _, kindName := range slices.Sorted(maps.Keys(r.kinds)) {
			belonging, _, err := r.resources.GetAll(ctx, kindName, resource.Filter{Parent: owner}, 0, 0)
			if err != nil {
				return nil, err
			}

			for _, d := range belonging {
				ref := graph.Ref{Kind: d.Kind, UUID: d.Metadata.UUID}

				if _, found := o.records[ref]; found || !slices.Contains(o.owners(d), owner) {
					continue
				}

				o.records[ref] = d
				waiting = append(waiting, ref)
			}
		}
	}

	for {
		var kept []graph.Ref

		for ref, d := range o.records {
			if ref == root {
				continue
			}

			stays, err := o.stays(ctx, d)
			if err != nil {
				return nil, err
			}

			if stays {
				kept = append(kept, ref)
			}
		}

		if len(kept) == 0 {
			return o, nil
		}

		for _, ref := range kept {
			delete(o.records, ref)
		}
	}
}

// stays reports whether something that stays owns d too: a record that is
// there, and is not among what goes.
func (o *owned) stays(ctx context.Context, d resource.Record) (bool, error) {
	for _, owner := range o.owners(d) {
		if _, goes := o.records[owner]; goes {
			continue
		}

		_, there, err := o.reconciler.read(ctx, owner)
		if err != nil || there {
			return there, err
		}
	}

	return false, nil
}

// owners are what d's record says owns it.
func (o *owned) owners(d resource.Record) []graph.Ref {
	return d.Metadata.Owning()
}

// levels are what goes, a level at a time, in the order it is removed in:
// each record once every one among what goes that owns it, or uses it, went
// before it, so that the root is first; and each level in the order of its
// kinds and its uuids. What owns or uses one another in a ring, which no
// order removes, goes last.
func (o *owned) levels() [][]graph.Ref {
	waits := make(map[graph.Ref]int, len(o.records))
	freed := make(map[graph.Ref][]graph.Ref, len(o.records))

	before := func(first graph.Ref, then graph.Ref) {
		if _, goes := o.records[first]; !goes || first == then {
			return
		}

		waits[then]++
		freed[first] = append(freed[first], then)
	}

	for ref, d := range o.records {
		for _, owner := range o.owners(d) {
			before(owner, ref)
		}

		for _, used := range d.Metadata.Refs {
			if _, goes := o.records[used]; goes {
				before(ref, used)
			}
		}
	}

	var levels [][]graph.Ref

	left := maps.Clone(o.records)

	for len(left) > 0 {
		var level []graph.Ref
		for ref := range left {
			if waits[ref] == 0 {
				level = append(level, ref)
			}
		}

		// a ring is removed as it is, last.
		if len(level) == 0 {
			level = slices.Collect(maps.Keys(left))
		}

		slices.SortFunc(level, func(a, b graph.Ref) int {
			return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.UUID, b.UUID))
		})

		for _, ref := range level {
			delete(left, ref)

			for _, then := range freed[ref] {
				waits[then]--
			}
		}

		levels = append(levels, level)
	}

	return levels
}
