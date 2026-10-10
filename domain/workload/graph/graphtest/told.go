package graphtest

import (
	"context"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Told is what carries the roots out as a test sees it (graph.Graphs): it
// keeps every record it is told changed and every root it is told to queue,
// in the order it is told, and does nothing else. Told a change while Failure
// is set, it keeps it, and fails with Failure.
type Told struct {
	lock    sync.Mutex
	changed []graph.Ref
	queued  []graph.Ref

	// Failure, when set, is what Changed fails with.
	Failure error
}

var _ graph.Graphs = &Told{}

func (t *Told) Changed(_ context.Context, ref graph.Ref) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.changed = append(t.changed, ref)

	return t.Failure
}

func (t *Told) Enqueue(root graph.Ref) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.queued = append(t.queued, root)
}

// Changes are the records it was told changed, in order.
func (t *Told) Changes() []graph.Ref {
	t.lock.Lock()
	defer t.lock.Unlock()

	return slices.Clone(t.changed)
}

// Queued are the roots it was told to queue, in order.
func (t *Told) Queued() []graph.Ref {
	t.lock.Lock()
	defer t.lock.Unlock()

	return slices.Clone(t.queued)
}
