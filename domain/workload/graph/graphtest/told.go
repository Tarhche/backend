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
// is set, it keeps it, and fails with Failure. Asked which roots need a
// record, it says what Depending says of it, and keeps that it was asked.
type Told struct {
	lock    sync.Mutex
	changed []graph.Ref
	queued  []graph.Ref
	asked   []graph.Ref

	// Failure, when set, is what Changed fails with.
	Failure error

	// Depending are the roots that need each record, as Dependents says
	// them, and DependentsFailure, when set, what Dependents fails with.
	Depending         map[graph.Ref][]graph.Dependent
	DependentsFailure error
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

func (t *Told) Dependents(_ context.Context, ref graph.Ref) ([]graph.Dependent, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.asked = append(t.asked, ref)

	if t.DependentsFailure != nil {
		return nil, t.DependentsFailure
	}

	return slices.Clone(t.Depending[ref]), nil
}

// Asked are the records it was asked which roots need, in order.
func (t *Told) Asked() []graph.Ref {
	t.lock.Lock()
	defer t.lock.Unlock()

	return slices.Clone(t.asked)
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
