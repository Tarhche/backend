// Package execute carries out the plan of what a root needs (graph.Plan): it
// makes what is not there, from the bottom up, a level at a time (Apply),
// removes the root and what it owns, from the top down (Remove), and says
// what either would do, without doing it (Preview, PreviewRemove).
//
// A level's nodes are made at once, as many at a time as the executor is
// let, and the next level is started once every node of it is done: each
// node is made after everything it needs, and handed the records they
// became. A node there already is done as it is, whatever made it, so a plan
// applied again makes nothing it made before, and one that stopped part way
// is taken up where it stopped. An external node is never made: while it is
// missing, it waits. Every node there, made or found, is linked to what it
// needs as the plan types each edge now (graph.Ownership.Link), so that the
// records keep what owns what as the policy says.
//
// A root is removed the other way, through the edges that own what they
// need and no other: what is loose, only ordered or external is never
// touched, and what something that stays owns stays too, and is unlinked
// from what goes (graph.Ownership). A node gone already is done as it is, so
// a delete that stopped part way is taken up where it stopped too.
//
// How each node went, the last time its root's plan was carried out, is kept
// (graph.Executions): pending until it is started, running while it is
// looked for and made or removed, and then done, waiting, or failed. A level
// with a node that waits or failed is the last one started: what is done
// stays done, nothing is undone, and the rest stay pending, for the next
// time.
package execute

import (
	"errors"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// DefaultConcurrency is how many nodes of a level are made or removed at
// once, unless an executor is told otherwise.
const DefaultConcurrency = 4

// Executor carries out plans on the kinds their nodes are of, knowing what
// owns their records, and keeping how each node went.
type Executor struct {
	kinds       graph.Kinds
	ownership   graph.Ownership
	executions  graph.Executions
	concurrency int
	now         func() time.Time
}

// Option changes how an executor goes about it.
type Option func(*Executor)

// Concurrency is how many nodes of a level are made or removed at once, at
// most.
func Concurrency(limit int) Option {
	return func(e *Executor) {
		e.concurrency = limit
	}
}

// New is an executor of plans of kinds, which knows what owns their records
// through ownership, keeps how each node went in executions, and makes or
// removes DefaultConcurrency nodes of a level at once, unless it is told
// otherwise; one told to make fewer than one is refused. A clock of nil is
// the time now.
func New(kinds graph.Kinds, ownership graph.Ownership, executions graph.Executions, now func() time.Time, options ...Option) (*Executor, error) {
	if now == nil {
		now = time.Now
	}

	e := &Executor{kinds: kinds, ownership: ownership, executions: executions, concurrency: DefaultConcurrency, now: now}

	for _, option := range options {
		option(e)
	}

	if e.concurrency < 1 {
		return nil, fmt.Errorf("an executor makes one node at a time at least, not %d", e.concurrency)
	}

	return e, nil
}

// levelsOf is the plan's levels, once every node of it is of a kind the
// executor is given: a plan with one that is not is carried out not at all,
// and is kind.ErrUnknownKind.
func (e *Executor) levelsOf(plan *graph.Plan) ([][]graph.NodeID, error) {
	if plan == nil {
		return nil, errors.New("there is no plan to carry out")
	}

	for _, node := range plan.Nodes() {
		if _, known := e.kinds[node.Kind]; !known {
			return nil, fmt.Errorf("%w: %q, the kind of %s", kind.ErrUnknownKind, node.Kind, node.ID())
		}
	}

	return plan.Levels(), nil
}
