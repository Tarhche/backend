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
// itself what it can be asked. Inside a parent the request names, a kind may
// name what it is asked of by more than its uuid (kind.Resolver).
//
// What is refused is said field by field, or, when what refused it is what a
// node would have refused it with, as the node's own refusal: a container that
// runs is not removed but by force, in Docker's words, before anything is asked
// of its node.
//
// A command that deletes the root of a graph, of a kind whose graph removes it
// (WithGraphs), marks it to be deleted rather than asking its node
// (dispatch.Dispatcher.Mark), and queues it to its graph, which removes it and
// then what it owns, in order: whoever asked may wait for it to go. Any other
// command carried out in the control plane on a root is queued to its graph
// once it is written down, which carries it out: a stack's start. One that
// deletes what is not a root is answered with the roots whose graphs need it,
// and whether one of them makes it again, and deletes them with it when it is
// asked to (Request.Cascade), as a delete does (deleteResource).
package actOnResource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
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

	// roots are the kinds whose roots their graphs remove, and graphs what
	// such a root is queued to once it is marked, and what says which roots
	// need what is not one; dependents finds, marks and waits on those.
	roots      graph.Roots
	graphs     graph.Graphs
	dependents *dependents.Dependents
}

// Option changes how commands are asked for.
type Option func(*UseCase)

// WithGraphs has a command that deletes a root of a kind roots names mark it
// to be deleted rather than ask its node, and queue it to graphs, whose graph
// removes it, and what it owns, in order; and one that deletes what is not a
// root answered with the roots graphs says need it, which a delete with them
// marks to go first.
func WithGraphs(roots graph.Roots, graphs graph.Graphs) Option {
	return func(uc *UseCase) {
		uc.roots = roots
		uc.graphs = graphs
	}
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher, logger *slog.Logger, options ...Option) *UseCase {
	uc := &UseCase{registry: registry, resources: resources, dispatcher: dispatcher, logger: logger}

	for _, option := range options {
		option(uc)
	}

	uc.dependents = dependents.New(registry, resources, dispatcher, uc.roots, uc.graphs)

	return uc
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

	r, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.extra(ctx, binding, request, uuid, err)
	} else if err != nil {
		return nil, err
	}

	var needs []graph.Dependent

	if action.Desires == kind.Deleted {
		if needs, err = uc.dependents.Of(ctx, d, r); err != nil {
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

	response, err := uc.act(ctx, binding, action, request, r, wait, cascaded)
	if err != nil || response == nil || len(needs) == 0 {
		return response, err
	}

	response.Dependents = needs
	response.Recreated = !cascaded && dependents.Recreated(needs)

	return response, nil
}

// act asks the resource request names for the command, and waits on it for
// as long as wait. first is its record as it was read, which is read again
// when something else wrote it since, or once what needed it was asked to go
// with it (cascaded), which may have taken it with them: it is gone then.
func (uc *UseCase) act(ctx context.Context, binding kind.ControlPlaneBinding, action kind.Action, request *Request, first resource.Record, wait time.Duration, cascaded bool) (*Response, error) {
	d := binding.Descriptor()

	var (
		asked  dispatch.Asked
		marked bool
	)

	r := first

	for try := 1; ; try++ {
		if try > 1 || cascaded {
			latest, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
			switch {
			case errors.Is(err, domain.ErrNotExists) && cascaded:
				return &Response{Gone: true}, nil
			case errors.Is(err, domain.ErrNotExists):
				return uc.extra(ctx, binding, request, uuid, err)
			case err != nil:
				return nil, err
			}

			r = latest
		}

		waits, err := uc.waits(action, d, r)
		if err != nil {
			return nil, err
		}

		var invalid domain.ValidationErrors

		marked = action.Desires == kind.Deleted && uc.roots.Root(d, r.Metadata)

		switch {
		case marked:
			asked, invalid, err = uc.dispatcher.Mark(ctx, binding, r, request.Action, request.Payload)
		case waits:
			asked = dispatch.Asked{}
			asked.Record, err = uc.dispatcher.Desire(ctx, r, action.Desires, true)
		default:
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

	if marked {
		return uc.marked(ctx, asked.Record, wait), nil
	}

	if action.Runs == kind.OnControlPlane && !asked.Gone {
		uc.carry(d, asked.Record)

		asked = uc.follow(ctx, binding, asked)
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

// carry queues r, the root of a graph carried out in the control plane, to
// its graph, once a command asked of it is written down on it: what it was
// asked, a stack's start say, is its graph's to carry out, now rather than at
// the next resync. What is no root is queued to nothing.
func (uc *UseCase) carry(d kind.Descriptor, r resource.Record) {
	if uc.graphs != nil && uc.roots.Root(d, r.Metadata) {
		uc.graphs.Enqueue(graph.Ref{Kind: r.Kind, UUID: r.Metadata.UUID})
	}
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
// one whoever asks may see. notThere is what looking for a record came to,
// which is the answer otherwise.
func (uc *UseCase) extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, uuid string, notThere error) (*Response, error) {
	extras, r, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, notThere)
	if err != nil {
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
// valid is said field by field; a command for a node, asked of a resource on
// none, as the node error of one that cannot be reached; and what was refused
// as its node would have refused it, as that node error.
func Refused(invalid domain.ValidationErrors, err error) (*Response, error) {
	var refused *noderequest.Error

	switch {
	case errors.Is(err, kind.ErrInvalidPayload):
		return &Response{ValidationErrors: domain.ValidationErrors{"payload": "invalid_value"}}, nil
	case errors.Is(err, kind.ErrUnreachable):
		return &Response{NodeError: &noderequest.Error{Code: noderequest.CodeNotRunning, Message: err.Error()}}, nil
	case errors.As(err, &refused):
		return &Response{NodeError: refused}, nil
	case err != nil:
		return nil, err
	case len(invalid) > 0:
		return &Response{ValidationErrors: invalid}, nil
	}

	return nil, nil
}
