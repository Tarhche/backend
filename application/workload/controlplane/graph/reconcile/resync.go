package reconcile

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Resync queues every root of every kind of root, as each kind's records
// say, a batch at a time: what nothing owns, but what it lives in (a record
// kept before refs has it among its owners). It catches up with whatever no
// change was told of, as a safety net, every so often.
//
// Then it removes what was owned by records that are all gone (collect): the
// end of a delete cut short after its root went.
//
// With no kinds of root, it reads nothing and does nothing. What fails with
// one kind is what it fails with, once every kind is done.
func (r *Reconciler) Resync(ctx context.Context) error {
	if len(r.roots) == 0 {
		return nil
	}

	var failed error

	for _, kindName := range slices.Sorted(maps.Keys(r.roots)) {
		failed = errors.Join(failed, r.queueRoots(ctx, kindName))
	}

	return errors.Join(failed, r.collect(ctx))
}

// queueRoots queues every root of the named kind, a batch at a time.
func (r *Reconciler) queueRoots(ctx context.Context, kindName string) error {
	unowned := resource.Filter{Unowned: &resource.Unowned{Parent: r.parentOf(kindName)}}

	for offset := uint(0); ; offset += r.config.Batch {
		roots, total, err := r.resources.GetAll(ctx, kindName, unowned, offset, r.config.Batch)
		if err != nil {
			return err
		}

		for _, root := range roots {
			r.Enqueue(graph.Ref{Kind: root.Kind, UUID: root.Metadata.UUID})
		}

		if len(roots) == 0 || offset+r.config.Batch >= total {
			return nil
		}
	}
}

// collect removes, through its kind (graph.Kind.Delete), each record of a
// kind a graph is made of that something owned, as its metadata says, and
// every one of whose owners is gone: what its owners' plans never got to
// remove, or what a root that went left. What it lives in owns nothing, so a
// record kept before refs, which has it among its owners, is never removed
// for its sake. One something still owns is kept, until a later collect finds
// what owns it gone: a network a container that is going owns goes once the
// container has. One whose kind cannot remove it now waits for a later
// collect too.
//
// What fails is logged, and is what it fails with, once every kind is done.
func (r *Reconciler) collect(ctx context.Context) error {
	var failed error

	for _, kindName := range slices.Sorted(maps.Keys(r.kinds)) {
		failed = errors.Join(failed, r.collectKind(ctx, kindName))
	}

	return failed
}

// collectKind removes each record of the named kind whose owners are all
// gone: those found, once its records are read, so that what is removed
// shifts nothing that is yet to be read.
func (r *Reconciler) collectKind(ctx context.Context, kindName string) error {
	parent := r.parentOf(kindName)

	var (
		orphans []graph.Ref
		failed  error
	)

	for offset := uint(0); ; offset += r.config.Batch {
		records, total, err := r.resources.GetAll(ctx, kindName, resource.Filter{}, offset, r.config.Batch)
		if err != nil {
			return errors.Join(failed, err)
		}

		for _, rec := range records {
			owners := rec.Metadata.Owning(parent)
			if len(owners) == 0 {
				continue
			}

			gone, err := r.allGone(ctx, owners)
			switch {
			case err != nil:
				r.logger.ErrorContext(ctx, "could not tell whether what owned a record is gone", "error", err, "kind", kindName, "uuid", rec.Metadata.UUID)
				failed = errors.Join(failed, err)
			case gone:
				orphans = append(orphans, graph.Ref{Kind: rec.Kind, UUID: rec.Metadata.UUID})
			}
		}

		if len(records) == 0 || offset+r.config.Batch >= total {
			break
		}
	}

	for _, orphan := range orphans {
		var waiting *graph.Waiting

		switch err := r.kinds[kindName].Delete(ctx, orphan); {
		case errors.As(err, &waiting):
			r.logger.DebugContext(ctx, "what nothing that owned it is left of waits to be removed", "kind", orphan.Kind, "uuid", orphan.UUID, "reason", waiting.Reason)
		case err != nil:
			r.logger.ErrorContext(ctx, "could not remove what nothing that owned it is left of", "error", err, "kind", orphan.Kind, "uuid", orphan.UUID)
			failed = errors.Join(failed, err)
		default:
			r.logger.InfoContext(ctx, "removed what nothing that owned it is left of", "kind", orphan.Kind, "uuid", orphan.UUID)
		}
	}

	return failed
}

// allGone reports whether the record of every one of owners is gone.
func (r *Reconciler) allGone(ctx context.Context, owners []graph.Ref) (bool, error) {
	for _, owner := range owners {
		_, there, err := r.read(ctx, owner)
		if err != nil || there {
			return false, err
		}
	}

	return true, nil
}
