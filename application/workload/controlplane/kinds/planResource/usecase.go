// Package planResource says what admitting a resource, or deleting one, would
// do, as a dry run: nothing is kept, sent, marked, queued or removed.
//
// What would be admitted is what its kind's admission admits as a dry run
// (admitResource.Request.DryRun): refused as it would be, and otherwise as it
// would be kept and as its first command would leave it, under a uuid nothing
// holds. A root of a graph is answered with what carrying its graph out would
// do as everything is now (Graphs.PreviewCreate): which of what it needs is
// there, which would be made, and which, external and missing, would be
// waited on, level by level.
//
// What would be deleted is found as a delete finds it. A root of a graph is
// answered with what removing it would do (Graphs.PreviewRemove): what it
// would remove, unlink and keep, level by level. Anything else is answered
// with the roots whose graphs need it, and whether one of them would make it
// again, as a delete would be; and, deleted with them, with what removing
// each would do.
//
// Where the graphs of roots are not kept, nothing is a root, and nothing
// needs anything.
package planResource

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Graphs are what says which roots need a record, and what carrying a root's
// graph out, or removing it, would do now: the reconciler.
type Graphs interface {
	graph.Dependents

	// PreviewCreate is what carrying out the graph of root, a record kept or
	// not, would do now, read and not done.
	PreviewCreate(ctx context.Context, root kind.Raw) (execute.Preview, error)

	// PreviewRemove is what removing the root ref names, as its record is
	// kept, would do now, read and not done.
	PreviewRemove(ctx context.Context, root graph.Ref) (execute.Preview, error)
}

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	admission *admitResource.UseCase

	// roots are the kinds whose resources are roots of their own graphs, and
	// graphs what previews them, and says what needs what.
	roots  graph.Roots
	graphs Graphs
}

// NewUseCase is what says what admitting a resource through admission, or
// deleting one kept in resources, would do, the roots of the kinds roots
// names previewed by graphs. With no graphs, nothing is a root.
func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, admission *admitResource.UseCase, roots graph.Roots, graphs Graphs) *UseCase {
	if graphs == nil {
		roots = nil
	}

	return &UseCase{registry: registry, resources: resources, admission: admission, roots: roots, graphs: graphs}
}

// Execute says what admitting the resource, or deleting it, would do. A kind
// that is not run here, and a resource to delete that is not there or not the
// owner's, are errors: kind.ErrUnknownKind and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	if request.Delete {
		return uc.deleted(ctx, binding, request)
	}

	return uc.admitted(ctx, binding, request)
}

// admitted is what admitting the resource request asks for would do: what
// its kind would admit, and, of a root, what carrying out its graph would.
func (uc *UseCase) admitted(ctx context.Context, binding kind.ControlPlaneBinding, request *Request) (*Response, error) {
	admitted, err := uc.admission.Execute(ctx, &admitResource.Request{
		Kind:      request.Kind,
		OwnerUUID: request.OwnerUUID,
		Parent:    request.Parent,
		Manifest:  request.Manifest,
		DryRun:    true,
	})
	if err != nil {
		return nil, err
	}

	if len(admitted.ValidationErrors) > 0 {
		return &Response{ValidationErrors: admitted.ValidationErrors}, nil
	}

	response := &Response{Resource: admitted.Resource}

	if !uc.roots.Root(binding.Descriptor(), admitted.Resource.Metadata) {
		return response, nil
	}

	plan, err := uc.graphs.PreviewCreate(ctx, admitted.Resource)
	if err != nil {
		return nil, err
	}

	response.Plan = &plan

	return response, nil
}

// deleted is what deleting the resource request names would do: of a root,
// what removing it would; of anything else, the roots that need it, and,
// deleted with them, what removing each would.
func (uc *UseCase) deleted(ctx context.Context, binding kind.ControlPlaneBinding, request *Request) (*Response, error) {
	r, _, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
	if err != nil {
		return nil, err
	}

	ref := graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID}

	if uc.roots.Root(binding.Descriptor(), r.Metadata) {
		plan, err := uc.graphs.PreviewRemove(ctx, ref)
		if err != nil {
			return nil, err
		}

		return &Response{Plan: &plan}, nil
	}

	if uc.graphs == nil {
		return &Response{}, nil
	}

	needs, err := uc.graphs.Dependents(ctx, ref)
	if err != nil || len(needs) == 0 {
		return &Response{}, err
	}

	response := &Response{
		Dependents: make([]Dependent, len(needs)),
		Recreated:  !request.Cascade && dependents.Recreated(needs),
	}

	for i, need := range needs {
		response.Dependents[i] = Dependent{Dependent: need}

		if !request.Cascade {
			continue
		}

		plan, err := uc.graphs.PreviewRemove(ctx, need.Root)
		if err != nil {
			return nil, err
		}

		response.Dependents[i].Plan = &plan
	}

	return response, nil
}
