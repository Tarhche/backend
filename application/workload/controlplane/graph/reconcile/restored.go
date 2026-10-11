package reconcile

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// restored is what becomes of root, whose record, rec, a restore of what it
// lives in reset, as its kind finds it in the restored parent
// (graph.ResetKind), and is its record once root is held there, to be carried
// out as any root is, or else how the run ended, and whether root is gone:
//
//   - held there, it is no longer reset, written so over the version its
//     record was read at, and carried out, making again what of it the
//     parent does not hold;
//   - not held, it is forgotten, with what it owns, under its lease (lost);
//   - and not known yet, it waits, and nothing of its plan is carried out,
//     so that nothing of it is made beside what the restored parent may hold
//     of it.
//
// A record written in the meantime, by what a person asked of the root say,
// or by another restore of its parent, is read again, and what its kind
// finds is asked again. What telling which it is, or writing it, fails with
// is a failure of what the control plane keeps (errRecords).
func (r *Reconciler) restored(ctx context.Context, root graph.Ref, resetting graph.ResetKind, rec resource.Record) (*resource.Record, graph.Ended, bool, error) {
	parent := r.parentOf(root.Kind)

	for try := 1; ; try++ {
		restoration, err := resetting.Restored(ctx, rec.Raw)
		if err != nil {
			ended, gone, err := r.failed(fmt.Errorf("%w: what its restored %s holds of it cannot be told: %w", errRecords, parent, err))

			return nil, ended, gone, err
		}

		switch restoration {
		case graph.RestorationLost:
			var (
				ended graph.Ended
				gone  bool
			)

			r.held(ctx, root, func(ctx context.Context) {
				ended, gone, err = r.lost(ctx, root, rec)
			})

			return nil, ended, gone, err

		case graph.RestorationHeld:
			held := rec
			held.Reset = false

			written, err := r.resources.Update(ctx, held)

			switch {
			case err == nil:
				r.logger.InfoContext(ctx, "a root its parent's restore reset is held by the restored parent, and carried out as before", "kind", root.Kind, "uuid", root.UUID)

				return &written, graph.Ended{}, false, nil
			case errors.Is(err, domain.ErrNotExists):
				r.logger.InfoContext(ctx, "forgetting a root that went as what its restored parent holds of it was told, its lease and how its plan went", "kind", root.Kind, "uuid", root.UUID)

				return nil, graph.Ended{}, true, nil
			case !errors.Is(err, resource.ErrConflict) || try >= tries:
				ended, gone, err := r.failed(fmt.Errorf("%w: it cannot be kept as its restored %s holds it: %w", errRecords, parent, err))

				return nil, ended, gone, err
			}

			read, there, err := r.read(ctx, root)

			switch {
			case err != nil:
				ended, gone, err := r.failed(fmt.Errorf("%w: %w", errRecords, err))

				return nil, ended, gone, err
			case !there:
				return nil, graph.Ended{}, true, nil
			case !read.Reset:
				return &read, graph.Ended{}, false, nil
			}

			rec = read

			continue
		}

		r.logger.DebugContext(ctx, "a root its parent's restore reset waits to be found in the restored parent", "kind", root.Kind, "uuid", root.UUID)

		return nil, graph.Ended{State: graph.StateWaiting, Reason: fmt.Sprintf("its %s was restored, and what the restored %s holds of it is yet to be known", parent, parent)}, false, nil
	}
}

// resets reports whether the roots of the named kind are reset by a restore
// of what they live in, and wait to be found in the restored parent
// (graph.ResetKind).
func (r *Reconciler) resets(kindName string) bool {
	_, resets := r.roots[kindName].(graph.ResetKind)

	return resets
}

// lost forgets root, whose record, rec, a restore of what it lives in reset,
// and which the restored parent does not hold, and what it owns, as what a
// restored parent does not hold is forgotten: their records go, and nothing
// is asked of a node, since what the restored parent does not hold is not
// there to remove. What it owns is what a removal through records finds
// (owning): what has root among its owners, what has one of those among its
// own, and so on, but what something that stays owns too, which is kept, and
// so then is what it owns. What it only uses, its VM and its volumes, keeps
// to its own kind's rules. What it owns that the restored parent holds after
// all, what a snapshot taken while it was being made has of it, say, is
// forgotten with it, and is what nobody keeps a record of from then on, shown
// as nobody's.
//
// Root's record goes last, so that a forget cut short leaves it to the next
// run, which finds it lost again; and a record found gone, forgotten by
// another control plane or by its kind's reset, is forgotten already. Once
// its record is gone, root is, and its lease and how its plan went are
// forgotten as a gone root's are. A run cut short, its lease lost or its
// control plane stopping, forgets nothing more.
func (r *Reconciler) lost(ctx context.Context, root graph.Ref, rec resource.Record) (graph.Ended, bool, error) {
	owned, err := r.owning(ctx, root, rec)

	switch {
	case err != nil && ctx.Err() != nil:
		return r.cutShort(ctx, "looking for what "+root.Kind+"/"+root.UUID+" owns")
	case err != nil:
		return r.failed(fmt.Errorf("%w: what it owns cannot be looked for: %w", errRecords, err))
	}

	forget := func(ref graph.Ref) (graph.Ended, bool, error) {
		if ctx.Err() != nil {
			return r.cutShort(ctx, "before forgetting "+ref.Kind+"/"+ref.UUID)
		}

		switch err := r.resources.Delete(ctx, ref.Kind, ref.UUID); {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return graph.Ended{}, false, nil
		case ctx.Err() != nil:
			return r.cutShort(ctx, "forgetting "+ref.Kind+"/"+ref.UUID)
		default:
			return r.failed(fmt.Errorf("%w: %s/%s cannot be forgotten: %w", errRecords, ref.Kind, ref.UUID, err))
		}
	}

	for _, level := range owned.levels() {
		for _, ref := range level {
			if ref == root {
				continue
			}

			if ended, gone, err := forget(ref); err != nil {
				return ended, gone, err
			}
		}
	}

	if ended, gone, err := forget(root); err != nil {
		return ended, gone, err
	}

	r.logger.InfoContext(ctx, "forgot a root its restored parent does not hold, and what it owned, as what the restored parent does not hold is", "kind", root.Kind, "uuid", root.UUID, "owned", len(owned.records)-1)

	return graph.Ended{}, true, nil
}
