// Package actOnResource asks a resource of any kind for one of its kind's
// commands, on somebody's behalf.
//
// The command has to be one somebody may ask for, which an internal one is
// not, and allowed in the state the resource is in. What it desires becomes
// what the resource is expected to be; then it is carried out in the control
// plane, or sent to the node holding the resource, and, when asked to, waited
// for.
//
// A command that waits (kind.Action.Waits), asked of a resource in flight,
// is not refused: what it desires is written down, and it is asked once the
// resource has got where it was going, so that what was asked after a
// command is carried out after it. One carried out in the control plane is
// followed at once by what the resource's kind asks for of it as it now is:
// a VM whose ports were changed is reconfigured on its node then, rather
// than on the reconcile loop's next pass.
//
// A uuid that names none of the kind's records may name one of its extras,
// such as one of the code runner's runs among anybody's VMs, which says for
// itself what it can be asked.
package actOnResource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	logger     *slog.Logger
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher, logger *slog.Logger) *UseCase {
	return &UseCase{registry: registry, resources: resources, dispatcher: dispatcher, logger: logger}
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
		if errors.Is(err, domain.ErrNotExists) {
			return uc.extra(ctx, binding, request, err)
		} else if err != nil {
			return nil, err
		}

		waits, err := uc.waits(action, d, r)
		if err != nil {
			return nil, err
		}

		var invalid domain.ValidationErrors

		if waits {
			asked = dispatch.Asked{}
			asked.Record, err = uc.dispatcher.Desire(ctx, r, action.Desires, true)
		} else {
			asked, invalid, err = uc.dispatcher.Ask(ctx, binding, r, request.Action, request.Payload, true)
		}

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

	if action.Runs == kind.OnControlPlane && !asked.Gone {
		asked = uc.follow(ctx, binding, asked)
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

// waits reports whether a command waits for the resource to get where it is
// going rather than being asked now: one that waits, asked of a resource in
// flight that it is not allowed in. Nothing waits on a resource on its way to
// being deleted, which is going nowhere anything could be asked of it.
func (uc *UseCase) waits(action kind.Action, d kind.Descriptor, r resource.Record) (bool, error) {
	if !action.Waits || len(action.Desires) == 0 {
		return false, nil
	}

	common, err := r.Common()
	if err != nil {
		return false, err
	}

	return !d.Allows(action.Name, common.State) && d.Machine.IsInFlight(common.State) && common.Expected != kind.Deleted, nil
}

// follow asks a resource just changed in the control plane for what its kind
// asks of it as it now is. What could not be asked is the reconcile loop's to
// ask again, and is no reason to answer the change it followed with a
// failure.
func (uc *UseCase) follow(ctx context.Context, binding kind.ControlPlaneBinding, asked dispatch.Asked) dispatch.Asked {
	followed, err := uc.dispatcher.Follow(ctx, binding, asked.Record)
	if err != nil {
		uc.logger.ErrorContext(ctx, "could not ask a resource just changed for what its kind asks", "error", err, "kind", asked.Record.Kind, "uuid", asked.Record.Metadata.UUID)
	}

	return followed
}

// extra asks one of the kind's extras for the command, when the uuid names
// one and whoever asks may see anybody's. notThere is what looking for a
// record came to, which is the answer otherwise.
func (uc *UseCase) extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, notThere error) (*Response, error) {
	extras, extended := binding.Extras()
	if !extended || len(request.OwnerUUID) > 0 {
		return nil, notThere
	}

	r, err := extras.One(ctx, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, notThere
	} else if err != nil {
		return nil, err
	}

	after, gone, refused, err := extras.Act(ctx, r, request.Action, request.Payload)
	if response, failed := Refused(refused, err); response != nil || failed != nil {
		return response, failed
	}

	if gone {
		return &Response{Gone: true}, nil
	}

	return &Response{Resource: after}, nil
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
