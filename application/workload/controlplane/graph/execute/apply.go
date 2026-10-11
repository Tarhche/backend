package execute

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Apply makes what plan needs, as the plan of the generation of root, and is
// what that came to.
//
// Every node of the plan is saved pending first. Then each level is carried
// out in turn, its nodes at once, as many at a time as the executor is let:
// a node is saved running, and looked for. One there already is done, as the
// record it is; one that is not, and is external, waits, and is never made;
// and any other is made, handed the records the nodes it needs became, and
// is done as the record it was made, or waits on what its kind says it waits
// on (graph.Waiting), or failed. Each is saved as it ends. A level with a
// node that waits or failed is the last one started.
//
// A node there, found or made, is linked to each node it needs, as the plan
// types the edge between them (graph.Ownership.Link), so that their records
// say what owns what and what uses what as the policy says now: applied
// again, a plan brings records kept under another policy in line with this
// one. Nothing external is owned: an edge that would own one, or own through
// one, is linked as loose. A link that fails fails its node, saying so, and
// the next run links it again.
//
// A run whose context is cancelled starts nothing more, waits for what runs
// to end, and saves each node it cut short as pending again, saying it was
// interrupted: it is not done, nor did it fail. Whatever becomes of the
// context, how each node went is saved.
//
// But a run of a plan carried out before at this generation, every node of
// which is kept, saves nothing while it finds each node as the run before
// left it: a root that is as it was is looked at again by every resync, and
// its plan says how the run that last found anything went, not that it is
// being looked at. Once it finds a node otherwise, ended in another state,
// for another reason or as another record, or is to make one, it saves all
// it would have saved so far, in order, and goes on as any run does; one cut
// short meanwhile changes nothing, and saves nothing, however soon: what is
// kept is read whatever becomes of its context, as what it saves is saved.
// One that cannot save all it held puts back what it saved of it as the run
// before left it.
//
// A plan with a node of a kind the executor is not given is carried out not
// at all, and is kind.ErrUnknownKind. What fails to be saved is an error,
// returned beside how far the plan went once every node started has ended:
// nothing more is started once a save has failed.
//
// What a run is to make it makes only while what options say it is made
// for holds (While).
func (e *Executor) Apply(ctx context.Context, root graph.Ref, generation int64, plan *graph.Plan, options ...ApplyOption) (Outcome, error) {
	levels, err := e.levelsOf(plan)
	if err != nil {
		return Outcome{}, err
	}

	r := e.newRun(plan)
	r.carry = r.create

	for _, option := range options {
		option(r)
	}

	r.quietly(ctx, root, generation, levels)

	now := e.now()

	err = r.start(ctx, levels, func(level int, node graph.Node) graph.Execution {
		return graph.Execution{
			Root:       root,
			Generation: generation,
			Node:       node.ID(),
			Kind:       node.Kind,
			Action:     graph.ActionCreate,
			Level:      level,
			State:      graph.StatePending,
			UpdatedAt:  now,
		}
	})
	if err != nil {
		return r.outcome(levels), err
	}

	r.carryOut(ctx, levels)

	return r.outcome(levels), r.err()
}

// ApplyOption changes how a plan is applied.
type ApplyOption func(*run)

// While has a run make what is not there only while wanted says nothing
// against it: it is asked once a node is found not to be there, before it is
// made, and what it says is what the node ends with, which makes nothing:
// a root asked to go, or gone, whose record a run read before it was, has
// nothing made for it after, what its delete took away among it.
func While(wanted func(ctx context.Context) error) ApplyOption {
	return func(r *run) {
		r.wanted = wanted
	}
}

// create finds the node, or makes it when it is not there, while what the run
// is for wants it made: an external one is never made, and waits while it is
// missing. It is the record the node is: one there that is not made, its
// record all the same, is made. Once it is there, it is linked to what it
// needs.
func (r *run) create(ctx context.Context, node graph.Node) (graph.Ref, string, error) {
	k := r.executor.kinds[node.Kind]

	ref, there, err := k.Exists(ctx, node.Spec)
	switch {
	case err != nil:
		return graph.Ref{}, "", err
	case !there && node.External:
		return graph.Ref{}, "", &graph.Waiting{Reason: "it is external, and missing"}
	case !there:
		if r.wanted != nil {
			if err := r.wanted(ctx); err != nil {
				return graph.Ref{}, "", err
			}
		}

		if err := r.began(ctx); err != nil {
			return graph.Ref{}, "", err
		}

		if ref, err = k.Create(ctx, node.Spec, r.deps(node.ID())); err != nil {
			return graph.Ref{}, "", err
		}
	}

	return ref, "", r.link(ctx, node, ref)
}

// quietly has a run of root's plan at generation save nothing (quiet) while
// it finds each node as a run before left it, when every node of the plan's
// levels is kept, at this generation, as a create at its level. A run that
// cannot tell saves everything, as any run does. What is kept is read
// whatever becomes of the run's context: a run cut short as it starts, its
// control plane stopping, would otherwise not tell, and save every node
// pending over a plan a run before carried out.
func (r *run) quietly(ctx context.Context, root graph.Ref, generation int64, levels [][]graph.NodeID) {
	kept, err := r.executor.executions.GetByRoot(context.WithoutCancel(ctx), root.UUID)
	if err != nil {
		return
	}

	byNode := make(map[graph.NodeID]graph.Execution, len(kept))
	for _, e := range kept {
		byNode[e.Node] = e
	}

	for level, ids := range levels {
		for _, id := range ids {
			if e, there := byNode[id]; !there || e.Generation != generation || e.Action != graph.ActionCreate || e.Level != level {
				return
			}
		}
	}

	r.kept = byNode
	r.quiet.Store(true)
}

// link links the node, the record ref, to each node it needs, in the order of
// their ids, as the plan types the edge between them: one that would own
// what is external, or own through what is, is linked as loose, since
// nothing external is owned. Each node it needs is in a level before its
// own, and done as the record it is.
func (r *run) link(ctx context.Context, node graph.Node, ref graph.Ref) error {
	for _, edge := range r.plan.Dependencies(node.ID()) {
		needed, _ := r.plan.Node(edge.From)

		t := edge.Type
		if t == graph.Owned && (needed.External || node.External) {
			t = graph.Loose
		}

		if err := r.executor.ownership.Link(ctx, r.execution(edge.From).Ref, ref, t); err != nil {
			return fmt.Errorf("it is there, and %s, which it needs, cannot be linked to it as %s: %w", edge.From, t, err)
		}
	}

	return nil
}

// deps are the records the nodes the node id needs became in this run, by
// their ids: each is in a level before its own, and done.
func (r *run) deps(id graph.NodeID) map[graph.NodeID]graph.Ref {
	dependencies := r.plan.Dependencies(id)

	r.lock.Lock()
	defer r.lock.Unlock()

	deps := make(map[graph.NodeID]graph.Ref, len(dependencies))
	for _, edge := range dependencies {
		deps[edge.From] = r.executions[edge.From].Ref
	}

	return deps
}
