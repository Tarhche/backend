// Package getPlan says how the plan of a root of a graph went, the last time
// it was carried out and found anything to say: how each of its nodes went
// (graph.Executions), which a run that finds each as it was says nothing of,
// and what its lease keeps of its failures (graph.Leases). A node the plan
// of a later generation of its root no longer has is kept as the last run of
// it left it, and is left out: only the nodes of the newest generation kept
// are said. What is no root, of a kind whose resources are not roots or
// owned by something, is not there, as what is not the owner's is not.
package getPlan

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

type UseCase struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	executions graph.Executions
	leases     graph.Leases

	// roots are the kinds whose resources are roots of their own graphs.
	roots graph.Roots
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, executions graph.Executions, leases graph.Leases, roots graph.Roots) *UseCase {
	return &UseCase{registry: registry, resources: resources, executions: executions, leases: leases, roots: roots}
}

// Execute is how the plan of the root request names went. A kind that is not
// run here, and a resource that is not there, not the owner's or no root,
// are errors: kind.ErrUnknownKind and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	r, _, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
	if err != nil {
		return nil, err
	}

	if !uc.roots.Root(binding.Descriptor(), r.Metadata) {
		return nil, domain.ErrNotExists
	}

	executions, err := uc.executions.GetByRoot(ctx, r.Metadata.UUID)
	if err != nil {
		return nil, err
	}

	response := &Response{
		Root:       graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID},
		Generation: max(r.Metadata.Generation, 1),
		Executions: newest(executions),
	}

	lease, err := uc.leases.Get(ctx, r.Metadata.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
	case err != nil:
		return nil, err
	default:
		response.Lease = &lease
	}

	return response, nil
}

// newest are those of executions of the newest generation among them, in
// their order: a node a later generation of its root's plan dropped is kept
// as the run of the one before left it, for good, and is no node of the plan
// as it is. Every run at a newer generation saves every node of its plan,
// each pending first, before it carries any out.
func newest(executions []graph.Execution) []graph.Execution {
	if len(executions) == 0 {
		return executions
	}

	var generation int64
	for _, e := range executions {
		generation = max(generation, e.Generation)
	}

	kept := make([]graph.Execution, 0, len(executions))
	for _, e := range executions {
		if e.Generation == generation {
			kept = append(kept, e)
		}
	}

	return kept
}
