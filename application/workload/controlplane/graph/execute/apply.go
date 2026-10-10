package execute

import (
	"context"

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
// A run whose context is cancelled starts nothing more, waits for what runs
// to end, and saves each node it cut short as pending again, saying it was
// interrupted: it is not done, nor did it fail. Whatever becomes of the
// context, how each node went is saved.
//
// A plan with a node of a kind the executor is not given is carried out not
// at all, and is kind.ErrUnknownKind. What fails to be saved is an error,
// returned beside how far the plan went once every node started has ended:
// nothing more is started once a save has failed.
func (e *Executor) Apply(ctx context.Context, root graph.Ref, generation int64, plan *graph.Plan) (Outcome, error) {
	levels, err := e.levelsOf(plan)
	if err != nil {
		return Outcome{}, err
	}

	r := e.newRun(plan)
	r.carry = r.create

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

// create finds the node, or makes it when it is not there: an external one
// is never made, and waits while it is missing. It is the record the node
// is: one there that is not made, its record all the same, is made.
func (r *run) create(ctx context.Context, node graph.Node) (graph.Ref, string, error) {
	k := r.executor.kinds[node.Kind]

	ref, there, err := k.Exists(ctx, node.Spec)
	switch {
	case err != nil:
		return graph.Ref{}, "", err
	case there:
		return ref, "", nil
	case node.External:
		return graph.Ref{}, "", &graph.Waiting{Reason: "it is external, and missing"}
	}

	ref, err = k.Create(ctx, node.Spec, r.deps(node.ID()))

	return ref, "", err
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
