package reconcile

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// PreviewCreate is what carrying out the graph of root would do now, read
// and not done (execute.Executor.Preview): root is a record of a kind of
// root, whether it is kept or only as its admission would keep it, and its
// graph is planned from what its kind says it needs (graph.RootKind.Root).
// Nothing is queued, made, saved or written, and no lease is taken. A record
// of a kind that is no root here is kind.ErrUnknownKind, and what planning
// it, or looking for its nodes, fails with is what it fails with.
func (r *Reconciler) PreviewCreate(ctx context.Context, root kind.Raw) (execute.Preview, error) {
	plan, err := r.planOf(root)
	if err != nil {
		return execute.Preview{}, err
	}

	return r.executor.Preview(ctx, plan)
}

// PreviewRemove is what removing the root ref names would do now, read and
// not done (execute.Executor.PreviewRemove), as its record is kept: what it
// removes, unlinks and keeps. Nothing is queued, removed, unlinked, saved or
// written, and no lease is taken. A root that is gone is domain.ErrNotExists,
// one of a kind that is no root here kind.ErrUnknownKind, and what planning
// it, or looking for its nodes and what owns them, fails with is what it
// fails with.
func (r *Reconciler) PreviewRemove(ctx context.Context, root graph.Ref) (execute.Preview, error) {
	if _, isRoot := r.roots[root.Kind]; !isRoot {
		return execute.Preview{}, fmt.Errorf("%w: %q is no kind of root here", kind.ErrUnknownKind, root.Kind)
	}

	rec, there, err := r.read(ctx, root)
	switch {
	case err != nil:
		return execute.Preview{}, err
	case !there:
		return execute.Preview{}, domain.ErrNotExists
	}

	plan, err := r.planOf(rec.Raw)
	if err != nil {
		return execute.Preview{}, err
	}

	return r.executor.PreviewRemove(ctx, plan)
}

// planOf is the plan of the graph of root, a record of a kind of root, as
// its kind says it needs.
func (r *Reconciler) planOf(root kind.Raw) (*graph.Plan, error) {
	rooted, isRoot := r.roots[root.Kind]
	if !isRoot {
		return nil, fmt.Errorf("%w: %q is no kind of root here", kind.ErrUnknownKind, root.Kind)
	}

	requirement, err := rooted.Root(root)
	if err != nil {
		return nil, fmt.Errorf("what the root needs cannot be read off its record: %w", err)
	}

	plan, err := r.planner.Plan(requirement)
	if err != nil {
		return nil, fmt.Errorf("its graph cannot be planned: %w", err)
	}

	return plan, nil
}
