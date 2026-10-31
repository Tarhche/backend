package reconcile

import (
	"context"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Enqueue queues root to be reconciled by a worker (Work): once, however
// often it is queued before a worker takes it up; and once more after it is
// reconciled when it is queued while it is, so that what changed meanwhile is
// not missed, and no two workers ever reconcile it at once. What is not of a
// kind of root here is not queued.
func (r *Reconciler) Enqueue(root graph.Ref) {
	if _, isRoot := r.roots[root.Kind]; !isRoot || len(root.UUID) == 0 {
		return
	}

	r.queue.add(root)
}

// Queued are the roots waiting for a worker, in the order they were queued,
// for a test to look at.
func (r *Reconciler) Queued() []graph.Ref {
	return r.queue.waiting()
}

// Work reconciles what is queued, as many roots at once as the reconciler
// has workers, until ctx is done, and returns once every worker has. A root
// a worker reconciles when ctx is done is cut short, and taken up later.
// What each comes to is logged where it is known (Reconcile).
func (r *Reconciler) Work(ctx context.Context) {
	var working sync.WaitGroup

	for range r.config.Workers {
		working.Go(func() {
			for {
				root, taken := r.queue.take(ctx)
				if !taken {
					return
				}

				_ = r.Reconcile(ctx, root)

				r.queue.done(root)
			}
		})
	}

	working.Wait()
}

// queue is the roots waiting for a worker, each once, in the order they were
// queued, and those being reconciled, none of which is taken up again until
// it is done, however often it is queued meanwhile.
type queue struct {
	lock sync.Mutex

	// order are the roots waiting, first first; queued those queued since a
	// worker last took each up, waiting or not; and reconciling those a
	// worker reconciles.
	order       []graph.Ref
	queued      map[graph.Ref]bool
	reconciling map[graph.Ref]bool

	// wake is told a root waits, once however often it is told before a
	// worker hears it.
	wake chan struct{}
}

func newQueue() *queue {
	return &queue{
		queued:      make(map[graph.Ref]bool),
		reconciling: make(map[graph.Ref]bool),
		wake:        make(chan struct{}, 1),
	}
}

// add queues root, unless it is queued already: one being reconciled waits
// until it is done.
func (q *queue) add(root graph.Ref) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.queued[root] {
		return
	}

	q.queued[root] = true

	if !q.reconciling[root] {
		q.order = append(q.order, root)
		q.signal()
	}
}

// take is the first root waiting, once there is one, which is reconciled
// from then until it is done, and whether there was one before ctx was done:
// none is taken once it is, and what waits waits on.
func (q *queue) take(ctx context.Context) (graph.Ref, bool) {
	for {
		if ctx.Err() != nil {
			return graph.Ref{}, false
		}

		q.lock.Lock()

		if len(q.order) > 0 {
			root := q.order[0]
			q.order = q.order[1:]

			delete(q.queued, root)
			q.reconciling[root] = true

			// another worker takes up the next.
			if len(q.order) > 0 {
				q.signal()
			}

			q.lock.Unlock()

			return root, true
		}

		q.lock.Unlock()

		select {
		case <-q.wake:
		case <-ctx.Done():
			return graph.Ref{}, false
		}
	}
}

// done is root reconciled: queued again meanwhile, it waits again.
func (q *queue) done(root graph.Ref) {
	q.lock.Lock()
	defer q.lock.Unlock()

	delete(q.reconciling, root)

	if q.queued[root] {
		q.order = append(q.order, root)
		q.signal()
	}
}

// waiting are the roots waiting, in order.
func (q *queue) waiting() []graph.Ref {
	q.lock.Lock()
	defer q.lock.Unlock()

	return slices.Clone(q.order)
}

// signal tells a worker a root waits. The lock is held.
func (q *queue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
