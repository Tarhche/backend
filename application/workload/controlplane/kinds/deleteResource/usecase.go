// Package deleteResource deletes a resource of any kind, by making deleted
// what it is expected to be.
//
// Its kind's delete is asked for at once when the state it is in allows it,
// with what it is asked with, a container that runs removed by force say,
// and is otherwise written down, to be asked for by the reconcile loop once
// it can be: a delete is a request to have it gone, and is never refused.
// The first one asked of it is when its delete was requested
// (kind.Metadata.DeletionRequestedAt), written beside its being expected
// deleted, in the same write. One that is being deleted already is left to
// it.
//
// The root of a graph, of a kind whose graph removes it (WithGraphs), is
// marked to be deleted rather than asked (dispatch.Dispatcher.Mark), its
// delete readied and refused as its node would refuse it, and queued to its
// graph, which removes it and then what it owns, in order: whoever asked may
// wait for it to go. Its graph's own delete of it is asked of its node as any
// delete is (Request.ByGraph).
//
// What is not a root is answered with the roots whose graphs need it
// (Response.Dependents), and whether one of them makes it again once it is
// gone, as a VM's containers make their VM again under its uuid. Deleted with
// them (Request.Cascade), each is marked to go first and queued to its graph
// (dependents.Dependents.Cascade): those that live in it go with it, as a
// VM's do, and it is asked for its delete at once; those that do not, which
// use a network or a volume, are waited on to go first, for as long as the
// request waits, or dependents.DefaultWait. Its node may still refuse its
// delete, when something still uses it, which leaves it as it was, and is
// answered with them all the same.
package deleteResource

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// deleteAction is the action every kind has that deletes its resources.
	deleteAction = "delete"

	// tries is how many times a resource is read and asked again when
	// something else wrote it in the meantime.
	tries = 3
)

type UseCase struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	dispatcher *dispatch.Dispatcher

	// roots are the kinds whose roots their graphs remove, and graphs what
	// such a root is queued to once it is marked, and what says which roots
	// need what is not one; dependents finds, marks and waits on those.
	roots      graph.Roots
	graphs     graph.Graphs
	dependents *dependents.Dependents
}

// Option changes how resources are deleted.
type Option func(*UseCase)

// WithGraphs has a root of a kind roots names marked to be deleted rather
// than asked, and queued to graphs, whose graph removes it, and what it owns,
// in order; and what is not a root answered with the roots graphs says need
// it, which a delete with them marks to go first.
func WithGraphs(roots graph.Roots, graphs graph.Graphs) Option {
	return func(uc *UseCase) {
		uc.roots = roots
		uc.graphs = graphs
	}
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher, options ...Option) *UseCase {
	uc := &UseCase{registry: registry, resources: resources, dispatcher: dispatcher}

	for _, option := range options {
		option(uc)
	}

	uc.dependents = dependents.New(registry, resources, dispatcher, uc.roots, uc.graphs)

	return uc
}

// Execute asks for it to be deleted, or marks a root its graph removes. A
// kind that is not run here, and a resource that is not there or not the
// owner's, are errors: kind.ErrUnknownKind and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	r, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return extra(ctx, binding, request, uuid, err)
	} else if err != nil {
		return nil, err
	}

	// a graph's own delete is answered with nothing of what needs it: what
	// needs it is what is removing it.
	var needs []graph.Dependent

	if !request.ByGraph {
		if needs, err = uc.dependents.Of(ctx, binding.Descriptor(), r); err != nil {
			return nil, err
		}
	}

	cascaded := request.Cascade && len(needs) > 0
	wait := request.Wait

	if cascaded {
		if wait, err = uc.dependents.Cascade(ctx, r, needs, wait); err != nil {
			return nil, err
		}
	}

	response, err := uc.delete(ctx, binding, request, r, wait, cascaded)
	if err != nil || len(needs) == 0 {
		return response, err
	}

	response.Dependents = needs
	response.Recreated = !cascaded && dependents.Recreated(needs)

	return response, nil
}

// delete asks for the resource request names to be deleted, or marks it, a
// root its graph removes, and waits on it for as long as wait. first is its
// record as it was read, which is read again when something else wrote it
// since, or once what needed it was asked to go with it (cascaded), which
// may have taken it with them: it is gone then.
func (uc *UseCase) delete(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, first resource.Record, wait time.Duration, cascaded bool) (*Response, error) {
	d := binding.Descriptor()

	r := first

	for try := 1; ; try++ {
		if try > 1 || cascaded {
			latest, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
			switch {
			case errors.Is(err, domain.ErrNotExists) && cascaded:
				return &Response{Gone: true}, nil
			case errors.Is(err, domain.ErrNotExists):
				return extra(ctx, binding, request, uuid, err)
			case err != nil:
				return nil, err
			}

			r = latest
		}

		common, err := r.Common()
		if err != nil {
			return nil, err
		}

		var (
			asked   dispatch.Asked
			invalid domain.ValidationErrors
			marked  bool
		)

		switch {
		case !request.ByGraph && uc.roots.Root(d, r.Metadata):
			asked, invalid, err = uc.dispatcher.Mark(ctx, binding, r, deleteAction, request.Payload)
			marked = true

		case common.Expected == kind.Deleted && r.Pending != nil && r.Pending.Action == deleteAction:
			return &Response{Resource: r.Raw}, nil

		case d.Allows(deleteAction, common.State):
			asked, invalid, err = uc.dispatcher.Ask(ctx, binding, r, deleteAction, request.Payload, true)

		default:
			asked.Record, err = uc.dispatcher.Desire(ctx, r, kind.Deleted, true)
		}

		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		} else if err != nil {
			return nil, err
		}

		if len(invalid) > 0 {
			return &Response{ValidationErrors: invalid}, nil
		}

		if marked {
			return uc.marked(ctx, asked.Record, wait), nil
		}

		delivered, err := uc.dispatcher.Deliver(ctx, asked, wait)
		if err != nil {
			return nil, err
		}

		return &Response{
			Resource: delivered.Resource,
			Gone:     delivered.Gone,
			Command:  delivered.Command,
			Result:   delivered.Result,
		}, nil
	}
}

// marked is a root marked to be deleted, queued to its graph, which removes
// it, and waited on for as long as wait until its graph has: gone, once it
// is, or as it was marked.
func (uc *UseCase) marked(ctx context.Context, r resource.Record, wait time.Duration) *Response {
	if uc.graphs != nil {
		uc.graphs.Enqueue(graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID})
	}

	if uc.dispatcher.Gone(ctx, r.Kind, r.Metadata.UUID, wait) {
		return &Response{Gone: true}
	}

	return &Response{Resource: r.Raw}
}

// extra deletes the kind's extra the uuid names, when whoever asks may see
// it, which takes it away as only it knows how. notThere is what looking for
// a record came to, which is the answer otherwise.
func extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, uuid string, notThere error) (*Response, error) {
	extras, r, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, notThere)
	if err != nil {
		return nil, err
	}

	after, gone, refused, err := extras.Act(ctx, r, deleteAction, request.Payload)
	switch {
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	case gone:
		return &Response{Gone: true}, nil
	}

	return &Response{Resource: after}, nil
}
