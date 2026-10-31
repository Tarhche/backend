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
//   - not held, it is forgotten, with what it owns, under its lease (lost),
//     once its kind finds it so again after what it owns was looked for;
//   - and not known yet, it waits, and nothing of its plan is carried out,
//     so that nothing of it is made beside what the restored parent may hold
//     of it.
//
// A record written in the meantime, by what a person asked of the root say,
// or by another restore of its parent, is read again, and what its kind
// finds is asked again, and so is one its kind finds otherwise as it is about
// to be forgotten. What telling which it is, or writing it, fails with is a
// failure of what the control plane keeps (errRecords).
func (r *Reconciler) restored(ctx context.Context, root graph.Ref, resetting graph.ResetKind, rec resource.Record) (*resource.Record, graph.Ended, bool, error) {
	parent := r.parentOf(root.Kind)

	// again reads root's record again, to be decided again, and reports
	// whether it is, still reset; or is what became of it otherwise: what
	// its record is now, no longer reset, or how the run ended.
	again := func() (bool, *resource.Record, graph.Ended, bool, error) {
		read, there, err := r.read(ctx, root)

		switch {
		case err != nil:
			ended, gone, err := r.failed(fmt.Errorf("%w: %w", errRecords, err))

			return false, nil, ended, gone, err
		case !there:
			return false, nil, graph.Ended{}, true, nil
		case !read.Reset:
			return false, &read, graph.Ended{}, false, nil
		}

		rec = read

		return true, nil, graph.Ended{}, false, nil
	}

	for try := 1; ; try++ {
		restoration, err := resetting.Restored(ctx, rec.Raw, rec.ResetAt)
		if err != nil {
			ended, gone, err := r.failed(fmt.Errorf("%w: what its restored %s holds of it cannot be told: %w", errRecords, parent, err))

			return nil, ended, gone, err
		}

		switch restoration {
		case graph.RestorationLost:
			var (
				ended           graph.Ended
				gone, otherwise bool
			)

			r.held(ctx, root, func(ctx context.Context) {
				ended, gone, otherwise, err = r.lost(ctx, root, resetting, rec)
			})

			if !otherwise {
				return nil, ended, gone, err
			}

			if try >= tries {
				return nil, graph.Ended{State: graph.StateWaiting, Reason: fmt.Sprintf("its %s was restored, and what the restored %s holds of it was found otherwise each time it was about to be forgotten", parent, parent)}, false, nil
			}

			if decide, held, ended, gone, err := again(); !decide {
				return held, ended, gone, err
			}

			continue

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

			if decide, held, ended, gone, err := again(); !decide {
				return held, ended, gone, err
			}

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
// and which its kind, resetting, found the restored parent does not hold, and
// what it owns, as what a restored parent does not hold is forgotten: their
// records go, and nothing is asked of a node, since what the restored parent
// does not hold is not there to remove. What it owns is what a removal
// through records finds (owning): what has root among its owners, what has
// one of those among its own, and so on, but what something that stays owns
// too, which is kept, and so then is what it owns. What it only uses, its VM
// and its volumes, keeps to its own kind's rules. What it owns that the
// restored parent holds after all, what a snapshot taken while it was being
// made has of it, say, is forgotten with it, and is what nobody keeps a
// record of from then on, shown as nobody's.
//
// Once what it owns is looked for, and before anything is forgotten, its
// record is read again, and its kind asked again: one written since it was
// found lost, by what a person asked of it or another restore of its parent,
// or one its kind finds otherwise now, something of it found in the restored
// parent since, or taken in from it, is forgotten not at all, and is decided
// again (otherwise). Root's record goes last, only at the version it was found
// lost at, so that a forget cut short leaves it to the next run, which finds
// it lost again, and one written meanwhile is decided again; and a record
// found gone, forgotten by another control plane or by its kind's reset, is
// forgotten already. What still names root among its owners once its record
// is gone, something taken in for it from the restored parent as it went, is
// forgotten after it, as what it owned was, rather than left owned by what is
// gone, which would have it removed from where it lives. Once its record is
// gone, root is, and its lease and how its plan went are forgotten as a gone
// root's are. A run cut short, its lease lost or its control plane stopping,
// forgets nothing more.
func (r *Reconciler) lost(ctx context.Context, root graph.Ref, resetting graph.ResetKind, rec resource.Record) (graph.Ended, bool, bool, error) {
	owns, err := r.owning(ctx, root, rec)

	switch {
	case err != nil && ctx.Err() != nil:
		ended, gone, err := r.cutShort(ctx, "looking for what "+root.Kind+"/"+root.UUID+" owns")

		return ended, gone, false, err
	case err != nil:
		ended, gone, err := r.failed(fmt.Errorf("%w: what it owns cannot be looked for: %w", errRecords, err))

		return ended, gone, false, err
	}

	// what it owns looked for, it is found lost again, on its record as it is
	// now, before anything is forgotten.
	now, there, err := r.read(ctx, root)

	switch {
	case err != nil && ctx.Err() != nil:
		ended, gone, err := r.cutShort(ctx, "reading "+root.Kind+"/"+root.UUID+" again")

		return ended, gone, false, err
	case err != nil:
		ended, gone, err := r.failed(fmt.Errorf("%w: %w", errRecords, err))

		return ended, gone, false, err
	case !there:
		return graph.Ended{}, true, false, nil
	case now.Version != rec.Version:
		r.logger.InfoContext(ctx, "a root found lost in its restored parent was written meanwhile, and is decided again", "kind", root.Kind, "uuid", root.UUID)

		return graph.Ended{}, false, true, nil
	}

	switch restoration, err := resetting.Restored(ctx, now.Raw, now.ResetAt); {
	case err != nil && ctx.Err() != nil:
		ended, gone, err := r.cutShort(ctx, "telling what the restored "+r.parentOf(root.Kind)+" holds of "+root.Kind+"/"+root.UUID+" again")

		return ended, gone, false, err
	case err != nil:
		ended, gone, err := r.failed(fmt.Errorf("%w: what its restored %s holds of it cannot be told: %w", errRecords, r.parentOf(root.Kind), err))

		return ended, gone, false, err
	case restoration != graph.RestorationLost:
		r.logger.InfoContext(ctx, "a root found lost in its restored parent is found otherwise once what it owns was looked for, and is decided again", "kind", root.Kind, "uuid", root.UUID, "restoration", restoration)

		return graph.Ended{}, false, true, nil
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

	forgetting := func(what *owned) (graph.Ended, bool, error) {
		for _, level := range what.levels() {
			for _, ref := range level {
				if ref == root {
					continue
				}

				if ended, gone, err := forget(ref); err != nil {
					return ended, gone, err
				}
			}
		}

		return graph.Ended{}, false, nil
	}

	if ended, gone, err := forgetting(owns); err != nil {
		return ended, gone, false, err
	}

	if ctx.Err() != nil {
		ended, gone, err := r.cutShort(ctx, "before forgetting "+root.Kind+"/"+root.UUID)

		return ended, gone, false, err
	}

	switch err := r.resources.DeleteAt(ctx, root.Kind, root.UUID, rec.Version); {
	case err == nil, errors.Is(err, domain.ErrNotExists):
	case errors.Is(err, resource.ErrConflict):
		r.logger.InfoContext(ctx, "a root found lost in its restored parent was written as it was being forgotten, and is decided again", "kind", root.Kind, "uuid", root.UUID)

		return graph.Ended{}, false, true, nil
	case ctx.Err() != nil:
		ended, gone, err := r.cutShort(ctx, "forgetting "+root.Kind+"/"+root.UUID)

		return ended, gone, false, err
	default:
		ended, gone, err := r.failed(fmt.Errorf("%w: %s/%s cannot be forgotten: %w", errRecords, root.Kind, root.UUID, err))

		return ended, gone, false, err
	}

	// what was taken in for it as it went is forgotten after it, as what it
	// owned was.
	left, err := r.owning(ctx, root, rec)
	if err == nil {
		_, _, err = forgetting(left)
	}

	if err != nil {
		r.logger.WarnContext(ctx, "could not forget what still names a root forgotten as its restored parent does not hold it among its owners", "error", err, "kind", root.Kind, "uuid", root.UUID)
	}

	r.logger.InfoContext(ctx, "forgot a root its restored parent does not hold, and what it owned, as what the restored parent does not hold is", "kind", root.Kind, "uuid", root.UUID, "owned", len(owns.records)-1)

	return graph.Ended{}, true, false, nil
}
