package execute

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// State is what carrying a plan out came to, as a whole.
type State string

const (
	// Done is every node of the plan done.
	Done State = "done"

	// Waiting is a node that waits on what it needs, which is there and
	// cannot take it now, or on an external node that is missing: its level
	// was the last one started.
	Waiting State = "waiting"

	// Failed is a node that failed: its level was the last one started.
	Failed State = "failed"

	// Interrupted is a run that stopped before every node was done, though
	// none failed or waits: it was cancelled, and what ran when it was is
	// pending again, or what it saved could not be.
	Interrupted State = "interrupted"
)

// Outcome is what carrying a plan out came to: what its root is reconciled
// by.
type Outcome struct {
	// State is what the plan came to as a whole: failed when a node failed,
	// whatever else waits; waiting when a node waits; interrupted when the
	// run stopped before every node was done; and done when every one is.
	State State

	// Nodes are how each node went, in the order the plan is carried out
	// in: level by level, and each level in the order of its nodes' ids.
	Nodes []graph.Execution

	// Err is what the first node that failed, in that order, failed with.
	Err error
}

// Node is how the node id names went, and whether the plan has it.
func (o Outcome) Node(id graph.NodeID) (graph.Execution, bool) {
	for _, e := range o.Nodes {
		if e.Node == id {
			return e, true
		}
	}

	return graph.Execution{}, false
}

// run is a plan being carried out, and how each node of it went so far.
type run struct {
	executor *Executor
	plan     *graph.Plan

	// carry is what is done to a node once it is saved running: it is the
	// record the node is, or was, and why it is done as it is, when that
	// wants saying, or else what stopped it.
	carry func(ctx context.Context, node graph.Node) (graph.Ref, string, error)

	// wanted, when there is one, says whether what is not there is still to
	// be made, as it is about to be (While).
	wanted func(ctx context.Context) error

	lock       sync.Mutex
	executions map[graph.NodeID]graph.Execution

	// failures are what each node that failed failed with, and unsaved what
	// the first save that failed failed with.
	failures map[graph.NodeID]error
	unsaved  error
}

// newRun is a run of plan, which does nothing to a node until it is told
// what to (carry).
func (e *Executor) newRun(plan *graph.Plan) *run {
	return &run{
		executor:   e,
		plan:       plan,
		executions: make(map[graph.NodeID]graph.Execution),
		failures:   make(map[graph.NodeID]error),
	}
}

// start keeps every node of the levels pending, as pending says it is at its
// level, and saves each, level by level, before any is started: a run whose
// nodes cannot all be saved is started not at all, and is what saving them
// failed with.
func (r *run) start(ctx context.Context, levels [][]graph.NodeID, pending func(level int, node graph.Node) graph.Execution) error {
	// every node is pending in the run before any is saved, so that a save
	// that fails leaves an outcome of every one.
	for level, ids := range levels {
		for _, id := range ids {
			node, _ := r.plan.Node(id)

			r.set(pending(level, node))
		}
	}

	for _, ids := range levels {
		for _, id := range ids {
			if err := r.save(ctx, r.execution(id)); err != nil {
				return err
			}
		}
	}

	return nil
}

// carryOut carries the levels out in turn, a level's nodes at once, as many
// at a time as the executor is let. A level with a node that is not done is
// the last one started, and none is started once the run is cancelled, or a
// save has failed.
func (r *run) carryOut(ctx context.Context, levels [][]graph.NodeID) {
	slots := make(chan struct{}, r.executor.concurrency)

	for _, ids := range levels {
		if ctx.Err() != nil {
			break
		}

		r.level(ctx, ids, slots)

		if !r.done(ids) {
			break
		}
	}
}

// level carries the nodes of one level out at once, as many at a time as
// there are slots, and returns once every one started has ended. None is
// started once the run is cancelled, or a save has failed.
func (r *run) level(ctx context.Context, ids []graph.NodeID, slots chan struct{}) {
	var running sync.WaitGroup

	for _, id := range ids {
		if !r.take(ctx, slots) {
			break
		}

		running.Go(func() {
			defer func() { <-slots }()

			r.node(ctx, id)
		})
	}

	running.Wait()
}

// take takes a slot for one more node, and reports whether it may start:
// none may once the run is cancelled, or a save has failed.
func (r *run) take(ctx context.Context, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return false
	}

	if ctx.Err() != nil || r.broken() {
		<-slots

		return false
	}

	return true
}

// node carries the node id out: it is saved running, has what the run does
// to a node done to it, and is saved as it ends.
func (r *run) node(ctx context.Context, id graph.NodeID) {
	node, _ := r.plan.Node(id)

	running := r.execution(id)
	running.State = graph.StateRunning
	running.StartedAt = r.executor.now()
	running.UpdatedAt = running.StartedAt

	// one not saved running is not started.
	if err := r.save(ctx, running); err != nil {
		return
	}

	r.set(running)

	ref, reason, err := r.carry(ctx, node)

	ended := running
	ended.UpdatedAt = r.executor.now()

	var waiting *graph.Waiting

	switch {
	case err == nil:
		ended.State = graph.StateDone
		ended.Reason = reason
		ended.Ref = ref
		ended.FinishedAt = ended.UpdatedAt
	case ctx.Err() != nil:
		// cancelled under it, it is neither done nor failed: it is left for
		// the next run, as it was before this one started it.
		ended.State = graph.StatePending
		ended.Reason = fmt.Sprintf("interrupted before it was done: %v", context.Cause(ctx))
		ended.StartedAt = time.Time{}
	case errors.As(err, &waiting):
		ended.State = graph.StateWaiting
		ended.Reason = waiting.Reason
		ended.FinishedAt = ended.UpdatedAt
	default:
		ended.State = graph.StateFailed
		ended.Reason = err.Error()
		ended.FinishedAt = ended.UpdatedAt

		r.failed(id, err)
	}

	r.set(ended)

	// what fails to be saved is the run's error, which save keeps.
	_ = r.save(ctx, ended)
}

// save saves e, whatever becomes of the run's context: what it says of its
// node is so, whether or not anybody still waits for it. The first save that
// fails is the run's error, and nothing more is started after it.
func (r *run) save(ctx context.Context, e graph.Execution) error {
	err := r.executor.executions.Save(context.WithoutCancel(ctx), e)
	if err == nil {
		return nil
	}

	err = fmt.Errorf("how %s went cannot be saved: %w", e.Node, err)

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.unsaved == nil {
		r.unsaved = err
	}

	return err
}

// execution is how the node id went so far.
func (r *run) execution(id graph.NodeID) graph.Execution {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.executions[id]
}

// set keeps e as how its node went so far.
func (r *run) set(e graph.Execution) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.executions[e.Node] = e
}

// failed keeps what the node id failed with.
func (r *run) failed(id graph.NodeID, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.failures[id] = err
}

// broken reports whether a save has failed.
func (r *run) broken() bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.unsaved != nil
}

// err is what the first save that failed failed with.
func (r *run) err() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.unsaved
}

// done reports whether every node of a level is done, and saved so: what
// the next level is started after.
func (r *run) done(ids []graph.NodeID) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.unsaved != nil {
		return false
	}

	for _, id := range ids {
		if r.executions[id].State != graph.StateDone {
			return false
		}
	}

	return true
}

// outcome is what the run came to, as far as it went.
func (r *run) outcome(levels [][]graph.NodeID) Outcome {
	r.lock.Lock()
	defer r.lock.Unlock()

	var failed, waits, undone bool

	outcome := Outcome{State: Done}

	for _, ids := range levels {
		for _, id := range ids {
			e := r.executions[id]
			outcome.Nodes = append(outcome.Nodes, e)

			switch e.State {
			case graph.StateDone:
			case graph.StateFailed:
				if !failed {
					outcome.Err = fmt.Errorf("%s: %w", id, r.failures[id])
				}

				failed = true
			case graph.StateWaiting:
				waits = true
			default:
				undone = true
			}
		}
	}

	switch {
	case failed:
		outcome.State = Failed
	case waits:
		outcome.State = Waiting
	case undone:
		outcome.State = Interrupted
	}

	return outcome
}
