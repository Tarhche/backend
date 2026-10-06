// Package deleteResource deletes a resource of any kind, by making deleted
// what it is expected to be.
//
// Its kind's delete is asked for at once when the state it is in allows it,
// and is otherwise written down, to be asked for by the reconcile loop once
// it can be: a delete is a request to have it gone, and is never refused.
// One that is being deleted already is left to it.
package deleteResource

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
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
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher) *UseCase {
	return &UseCase{registry: registry, resources: resources, dispatcher: dispatcher}
}

// Execute asks for it to be deleted. A kind that is not run here, and a
// resource that is not there or not the owner's, are errors:
// kind.ErrUnknownKind and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()

	for try := 1; ; try++ {
		r, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
		if errors.Is(err, domain.ErrNotExists) {
			return extra(ctx, binding, request, uuid, err)
		} else if err != nil {
			return nil, err
		}

		common, err := r.Common()
		if err != nil {
			return nil, err
		}

		var asked dispatch.Asked

		switch {
		case common.Expected == kind.Deleted && r.Pending != nil && r.Pending.Action == deleteAction:
			return &Response{Resource: r.Raw}, nil

		case d.Allows(deleteAction, common.State):
			asked, _, err = uc.dispatcher.Ask(ctx, binding, r, deleteAction, nil, true)

		default:
			asked.Record, err = uc.dispatcher.Desire(ctx, r, kind.Deleted, true)
		}

		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		} else if err != nil {
			return nil, err
		}

		delivered, err := uc.dispatcher.Deliver(ctx, asked, request.Wait)
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

// extra deletes the kind's extra the uuid names, when whoever asks may see
// it, which takes it away as only it knows how. notThere is what looking for
// a record came to, which is the answer otherwise.
func extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, uuid string, notThere error) (*Response, error) {
	extras, r, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, notThere)
	if err != nil {
		return nil, err
	}

	after, gone, refused, err := extras.Act(ctx, r, deleteAction, nil)
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
