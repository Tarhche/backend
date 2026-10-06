// Package queryResource asks a resource of any kind one of its kind's
// queries, and answers at once.
//
// A query a node answers is asked of the node holding the resource as a node
// request, as vm.logs already is: its op is the kind and the action,
// "fan.logs", and it carries the resource as it is recorded, so the node
// needs no database. The only query the control plane answers itself is the
// state of a kind whose state is known there, from its record.
//
// Asking a node for one resource's state is also an observation of it,
// outside the heartbeat, so it is written down as one: the dashboard
// refreshing one resource refreshes its record.
//
// A uuid that names none of the kind's records may name one of its extras,
// such as one of the code runner's runs among anybody's VMs, which answers
// for itself, and what its node refused it is said as that node's refusal.
// Inside a parent the request names, a kind may name what it is asked of by
// more than its uuid (kind.Resolver).
package queryResource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// stateAction is the query every kind answers with what a resource is doing.
const stateAction = "state"

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
	requester noderequest.Requester
	observer  *observe.Observer
	now       func() time.Time
}

// NewUseCase is a use case that asks the nodes through requester and writes
// down the states they answer with through observer. A clock of nil is the
// time now.
func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, requester noderequest.Requester, observer *observe.Observer, now func() time.Time) *UseCase {
	if now == nil {
		now = time.Now
	}

	return &UseCase{registry: registry, resources: resources, requester: requester, observer: observer, now: now}
}

// Execute asks the query. A kind that is not run here, a query it does not
// have, and a resource that is not there or not the owner's, are errors:
// kind.ErrUnknownKind, kind.ErrUnknownAction and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()

	action, found := d.Action(request.Action)
	if !found || action.Mode != kind.ModeQuery {
		return nil, fmt.Errorf("%w: a %s has no query %q", kind.ErrUnknownAction, d.Name, request.Action)
	}

	r, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.extra(ctx, binding, request, uuid, err)
	} else if err != nil {
		return nil, err
	}

	common, err := r.Common()
	if err != nil {
		return nil, err
	}

	if !d.Allows(request.Action, common.State) {
		return &Response{NodeError: notReachable("a %s that is %s cannot be asked for its %s", d.Name, common.State, request.Action)}, nil
	}

	_, invalid, err := action.Payload.Decode(request.Payload)
	switch {
	case errors.Is(err, kind.ErrInvalidPayload):
		return &Response{ValidationErrors: domain.ValidationErrors{"payload": "invalid_value"}}, nil
	case err != nil:
		return nil, err
	case len(invalid) > 0:
		return &Response{ValidationErrors: invalid}, nil
	}

	if action.Runs == kind.OnControlPlane {
		return uc.fromRecord(d, r)
	}

	payload, err := dispatch.Compact(request.Payload)
	if err != nil {
		return &Response{ValidationErrors: domain.ValidationErrors{"payload": "invalid_value"}}, nil
	}

	if len(r.Metadata.Node) == 0 {
		return &Response{NodeError: notReachable("the %s is on no node yet", d.Name)}, nil
	}

	asked, err := kind.Query{Kind: d.Name, UUID: r.Metadata.UUID, Action: request.Action, Payload: payload, Resource: r.Raw}.Request()
	if err != nil {
		return nil, err
	}

	reply, err := uc.requester.Request(ctx, r.Metadata.Node, asked)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &Response{NodeError: &noderequest.Error{Code: noderequest.CodeTimeout, Message: fmt.Sprintf("the node holding the %s did not answer in time", d.Name)}}, nil
	case err != nil:
		return &Response{NodeError: &noderequest.Error{Code: noderequest.CodeInternal, Message: fmt.Sprintf("the node holding the %s is not answering", d.Name)}}, nil
	}

	var refused *noderequest.Error
	if errors.As(reply.Err(), &refused) {
		return &Response{NodeError: refused}, nil
	}

	if request.Action == stateAction {
		uc.observed(ctx, d, r, reply.Result)
	}

	return &Response{Result: reply.Result, Truncated: reply.Truncated}, nil
}

// extra asks one of the kind's extras the query, when the uuid names one
// whoever asks may see: its state is what it says it is doing, in the shape a
// node answers one in, and anything else is its own to answer. notThere is
// what looking for a record came to, which is the answer otherwise.
func (uc *UseCase) extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, uuid string, notThere error) (*Response, error) {
	extras, r, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, notThere)
	if err != nil {
		return nil, err
	}

	if request.Action == stateAction {
		return uc.fromRecord(binding.Descriptor(), resource.Record{Raw: r})
	}

	var nodeRefused *noderequest.Error

	answer, refused, err := extras.Query(ctx, r, request.Action, request.Payload)
	switch {
	case errors.As(err, &nodeRefused):
		return &Response{NodeError: nodeRefused}, nil
	case err != nil:
		return nil, err
	case len(refused) > 0:
		return &Response{ValidationErrors: refused}, nil
	}

	return &Response{Result: answer}, nil
}

// fromRecord answers the state of a kind whose state is known in the
// control plane, from its record, in the shape a node answers one in.
func (uc *UseCase) fromRecord(d kind.Descriptor, r resource.Record) (*Response, error) {
	answer, err := json.Marshal(kind.Observation{Kind: d.Name, UUID: r.Metadata.UUID, Owners: r.Metadata.Owners, Status: r.Status})
	if err != nil {
		return nil, err
	}

	return &Response{Result: answer}, nil
}

// observed writes down what a node answered of a resource's state. What
// could not be written is the next heartbeat's to say again, and is no
// reason not to answer.
func (uc *UseCase) observed(ctx context.Context, d kind.Descriptor, r resource.Record, answer json.RawMessage) {
	if uc.observer == nil {
		return
	}

	var observation kind.Observation
	if err := json.Unmarshal(answer, &observation); err != nil || observation.UUID != r.Metadata.UUID {
		return
	}

	_ = uc.observer.Take(ctx, d, r.Metadata.Node, r, observation.Status, uc.now())
}

// notReachable is the answer about a resource that cannot be asked now.
func notReachable(format string, args ...any) *noderequest.Error {
	return &noderequest.Error{Code: noderequest.CodeNotRunning, Message: fmt.Sprintf(format, args...)}
}
