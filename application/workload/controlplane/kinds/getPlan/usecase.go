// Package getPlan says how the plan of a root of a graph went, the last time
// it was carried out and found anything to say: how each of its nodes went
// (graph.Executions), which a run that finds each as it was says nothing of,
// and what its lease keeps of its failures (graph.Leases). Every node kept is
// said, whatever generation of the plan it was saved at: a run of a later
// generation of its root forgets the nodes its plan no longer has once it has
// saved every node of its own (graph.Executions.DeleteBefore), and one that
// saved only part of it before it stopped leaves the rest as the run before
// left them, which are its plan's nodes still. So may the plan say nodes of
// two generations for a while: what a run could not forget, or did not, not
// reading what was kept as it began, and what a run that lost its root's
// lease saved of an older generation once a newer one forgot it, each
// forgotten by the next run that finds it kept. What is no root, of a kind
// whose resources are not roots or owned by something, is not there, as what
// is not the owner's is not.
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
		Generation: r.Metadata.Generation,
		Executions: executions,
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
