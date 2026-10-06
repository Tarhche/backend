// Package actOnResource asks a resource of any kind for one of its kind's
// commands, on somebody's behalf.
//
// The command has to be one somebody may ask for, which an internal one is
// not, and allowed in the state the resource is in. What it desires becomes
// what the resource is expected to be; then it is carried out in the control
// plane, or sent to the node holding the resource, and, when asked to, waited
// for.
package actOnResource

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// tries is how many times a resource is read and asked again when something
// else wrote it in the meantime.
const tries = 3

type UseCase struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	dispatcher *dispatch.Dispatcher
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher) *UseCase {
	return &UseCase{registry: registry, resources: resources, dispatcher: dispatcher}
}

// Execute asks for the command. A kind that is not run here, a command it
// does not have or nobody may ask for, and a resource that is not there or
// not the owner's, are errors: kind.ErrUnknownKind, kind.ErrUnknownAction and
// domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()

	action, found := d.Action(request.Action)
	if !found || action.Internal || action.Mode != kind.ModeCommand {
		return nil, fmt.Errorf("%w: a %s has no command %q", kind.ErrUnknownAction, d.Name, request.Action)
	}

	var asked dispatch.Asked

	for try := 1; ; try++ {
		r, err := owner.Resource(ctx, uc.resources, d.Name, request.OwnerUUID, request.UUID)
		if err != nil {
			return nil, err
		}

		var invalid domain.ValidationErrors

		asked, invalid, err = uc.dispatcher.Ask(ctx, binding, r, request.Action, request.Payload, true)
		if errors.Is(err, resource.ErrConflict) && try < tries {
			continue
		}

		if refused, failed := Refused(invalid, err); refused != nil || failed != nil {
			if failed != nil {
				return nil, failed
			}

			return refused, nil
		}

		break
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

// Refused is the response to asking that was refused, or the error it
// failed with, or neither when it was neither: what cannot be read or is not
// valid is said field by field, and a command for a node, asked of a
// resource on none, as the node error of one that cannot be reached.
func Refused(invalid domain.ValidationErrors, err error) (*Response, error) {
	switch {
	case errors.Is(err, kind.ErrInvalidPayload):
		return &Response{ValidationErrors: domain.ValidationErrors{"payload": "invalid_value"}}, nil
	case errors.Is(err, kind.ErrUnreachable):
		return &Response{NodeError: &noderequest.Error{Code: noderequest.CodeNotRunning, Message: err.Error()}}, nil
	case err != nil:
		return nil, err
	case len(invalid) > 0:
		return &Response{ValidationErrors: invalid}, nil
	}

	return nil, nil
}
