package kind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Binding is a kind's strategy in one service, together with the descriptor
// it was bound to. It is what a Registry holds.
//
// A strategy is typed, with its kind's own spec and status, and a registry
// holds kinds of every type, so a strategy is bound before it is registered:
// the Bind function of its service captures the types and gives back a
// binding that speaks Raw. Whatever arrives as JSON is read as the kind's own
// types there, at the edge, once, and what the strategy hands back is written
// as JSON there too.
//
// A binding does not ask whether an action is allowed in the state the
// resource is in. That is the dispatcher's to ask (Descriptor.Allows), once,
// before it moves the resource on: by the time a node carries a command out,
// the resource it carries may be on its way already, starting for a start,
// where a start is not allowed.
type Binding interface {
	Descriptor() Descriptor
}

// ControlPlaneBinding is a kind's control-plane strategy with its types
// erased: what the control plane's generic admission, dispatch and reconcile
// loop hold every kind as.
type ControlPlaneBinding interface {
	Binding

	// Admit is the strategy's Admit of a raw resource asked for, with its
	// kind filled in and no status: what a resource is doing is never the
	// caller's to say. A resource of another kind is refused as
	// ErrUnknownKind, and a spec that cannot be read as the kind's as
	// ErrInvalidPayload.
	Admit(ctx context.Context, asked Raw) (Raw, domain.ValidationErrors, error)

	// Reconcile is the strategy's Reconcile of a raw resource.
	Reconcile(ctx context.Context, r Raw) ([]Intent, error)

	// Apply decodes payload with the action's codec and is the strategy's
	// Apply of it. An action that is not a command run in the control plane
	// is refused as ErrUnknownAction; a payload that cannot be read is
	// ErrInvalidPayload, and one that is not valid is what is wrong with it.
	Apply(ctx context.Context, r Raw, action string, payload []byte) (Raw, domain.ValidationErrors, error)

	// Prepare readies one of the kind's node commands before it is sent,
	// through the strategy's Preparer, and is the resource the command
	// carries; a strategy that is not one leaves the resource as it is. An
	// action that is not a command run on a node is refused as
	// ErrUnknownAction; a payload that cannot be read is ErrInvalidPayload,
	// and one that is not valid is what is wrong with it.
	Prepare(ctx context.Context, r Raw, action string, payload []byte) (Raw, domain.ValidationErrors, error)

	// Extras are what the kind's listings show beside its records, when its
	// strategy is an Extender.
	Extras() (Extras, bool)

	// Witness is what hears everything the kind's reports say, when its
	// strategy is a Witness.
	Witness() (Witness, bool)

	// Resolver is what finds the kind's resources by what else names them
	// inside their parents, when its strategy is a Resolver.
	Resolver() (Resolver, bool)
}

// NodeBinding is a kind's node strategy with its types erased: what an
// orchestrator's dispatcher, request responder and heartbeat hold every kind
// as.
type NodeBinding interface {
	Binding

	// Execute carries out a command addressed to this node, and is what came
	// of it. It never fails: whatever went wrong, a command that is not the
	// kind's or a payload that cannot be read as much as a strategy that
	// failed, is the Result's Reason. Its At is left for whoever sends it.
	Execute(ctx context.Context, command Command) Result

	// Query answers a query: the strategy's answer as JSON, or, for state,
	// the resource's Observation, read off State. A resource the node does
	// not hold is observed Missing; one inside a parent the node could not
	// look into is ErrUnseen.
	Query(ctx context.Context, query Query) (json.RawMessage, error)

	// State is the strategy's State, every status as JSON and every
	// instance said to be of the kind.
	State(ctx context.Context) (Report[json.RawMessage], error)

	// Attaches reports whether the strategy serves stream actions, which is
	// whether it is an Attacher.
	Attaches() bool

	// Attach opens a stream action through the strategy's Attacher.
	Attach(ctx context.Context, action string, uuid string, owner string) (Session, error)

	// Exposes reports whether the strategy serves its kind's ports, which is
	// whether it is an Exposer.
	Exposes() bool

	// Prompt is how often the strategy is asked what it holds between beats,
	// when it is a Prompt, and nothing when it is not.
	Prompt() time.Duration

	// Endpoint is where a port of the instance slug names is reached,
	// through the strategy's Exposer.
	Endpoint(ctx context.Context, slug string, p port.Port) (Endpoint, error)
}

// IngressBinding is a kind's ingress strategy, held with its descriptor. It
// has no types to erase: where an instance is does not depend on its spec.
type IngressBinding interface {
	Binding
	Ingress
}

// BindControlPlane binds a kind's control-plane strategy to its descriptor.
// Its status has to embed Status, which is what lets the framework read the
// part of it every kind shares.
func BindControlPlane[Spec, Status any, S interface {
	*Status
	Stated
}](d Descriptor, strategy ControlPlane[Spec, Status]) ControlPlaneBinding {
	return &controlPlaneBinding[Spec, Status]{descriptor: d, strategy: strategy}
}

// BindNode binds a kind's node strategy to its descriptor. Its status has to
// embed Status, which is what lets the framework read the part of it every
// kind shares.
func BindNode[Spec, Status any, S interface {
	*Status
	Stated
}](d Descriptor, strategy Node[Spec, Status]) NodeBinding {
	return &nodeBinding[Spec, Status, S]{descriptor: d, strategy: strategy}
}

// BindIngress binds a kind's ingress strategy to its descriptor.
func BindIngress(d Descriptor, strategy Ingress) IngressBinding {
	return ingressBinding{Ingress: strategy, descriptor: d}
}

type controlPlaneBinding[Spec, Status any] struct {
	descriptor Descriptor
	strategy   ControlPlane[Spec, Status]
}

var _ ControlPlaneBinding = &controlPlaneBinding[struct{}, Status]{}

func (b *controlPlaneBinding[Spec, Status]) Descriptor() Descriptor {
	return b.descriptor
}

func (b *controlPlaneBinding[Spec, Status]) Admit(ctx context.Context, asked Raw) (Raw, domain.ValidationErrors, error) {
	// what the API was asked need not say what it is: the route it came to
	// does.
	if len(asked.Kind) == 0 {
		asked.Kind = b.descriptor.Name
	}

	asked.Status = nil

	typed, err := typedAs[Spec, Status](b.descriptor, asked)
	if errors.Is(err, ErrUnknownKind) {
		return Raw{}, nil, err
	} else if err != nil {
		return Raw{}, nil, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}

	admitted, invalid, err := b.strategy.Admit(ctx, typed)
	if err != nil || len(invalid) > 0 {
		return Raw{}, invalid, err
	}

	admitted.Kind = b.descriptor.Name

	raw, err := Encode(admitted)

	return raw, nil, err
}

func (b *controlPlaneBinding[Spec, Status]) Reconcile(ctx context.Context, r Raw) ([]Intent, error) {
	typed, err := typedAs[Spec, Status](b.descriptor, r)
	if err != nil {
		return nil, err
	}

	return b.strategy.Reconcile(ctx, typed)
}

func (b *controlPlaneBinding[Spec, Status]) Apply(ctx context.Context, r Raw, action string, payload []byte) (Raw, domain.ValidationErrors, error) {
	a, err := actionOf(b.descriptor, b.descriptor.Name, action, OnControlPlane, ModeCommand)
	if err != nil {
		return Raw{}, nil, err
	}

	value, invalid, err := a.Payload.Decode(payload)
	if err != nil || len(invalid) > 0 {
		return Raw{}, invalid, err
	}

	typed, err := typedAs[Spec, Status](b.descriptor, r)
	if err != nil {
		return Raw{}, nil, err
	}

	applied, invalid, err := b.strategy.Apply(ctx, typed, action, value)
	if err != nil || len(invalid) > 0 {
		return Raw{}, invalid, err
	}

	applied.Kind = b.descriptor.Name

	raw, err := Encode(applied)

	return raw, nil, err
}

func (b *controlPlaneBinding[Spec, Status]) Prepare(ctx context.Context, r Raw, action string, payload []byte) (Raw, domain.ValidationErrors, error) {
	a, err := actionOf(b.descriptor, b.descriptor.Name, action, OnNode, ModeCommand)
	if err != nil {
		return Raw{}, nil, err
	}

	preparer, prepares := b.strategy.(Preparer[Spec, Status])
	if !prepares {
		return r, nil, nil
	}

	value, invalid, err := a.Payload.Decode(payload)
	if err != nil || len(invalid) > 0 {
		return Raw{}, invalid, err
	}

	typed, err := typedAs[Spec, Status](b.descriptor, r)
	if err != nil {
		return Raw{}, nil, err
	}

	prepared, invalid, err := preparer.Prepare(ctx, typed, action, value)
	if err != nil || len(invalid) > 0 {
		return Raw{}, invalid, err
	}

	prepared.Kind = b.descriptor.Name

	raw, err := Encode(prepared)

	return raw, nil, err
}

func (b *controlPlaneBinding[Spec, Status]) Extras() (Extras, bool) {
	extender, extends := b.strategy.(Extender)
	if !extends {
		return nil, false
	}

	extras := extender.Extras()

	return extras, extras != nil
}

func (b *controlPlaneBinding[Spec, Status]) Witness() (Witness, bool) {
	witness, witnesses := b.strategy.(Witness)

	return witness, witnesses
}

func (b *controlPlaneBinding[Spec, Status]) Resolver() (Resolver, bool) {
	resolver, resolves := b.strategy.(Resolver)

	return resolver, resolves
}

type nodeBinding[Spec, Status any, S interface {
	*Status
	Stated
}] struct {
	descriptor Descriptor
	strategy   Node[Spec, Status]
}

var _ NodeBinding = &nodeBinding[struct{}, Status, *Status]{}

func (b *nodeBinding[Spec, Status, S]) Descriptor() Descriptor {
	return b.descriptor
}

func (b *nodeBinding[Spec, Status, S]) Execute(ctx context.Context, command Command) Result {
	result := Result{
		ID:      command.ID,
		Kind:    command.Kind,
		UUID:    command.UUID,
		Action:  command.Action,
		Node:    command.Node,
		Attempt: command.Attempt,
	}

	failed := func(err error) Result {
		result.OK = false
		result.Refused = errors.Is(err, ErrRefused)
		result.Reason = err.Error()

		return result
	}

	a, err := actionOf(b.descriptor, command.Kind, command.Action, OnNode, ModeCommand)
	if err != nil {
		return failed(err)
	}

	value, err := decodePayload(a, command.Payload)
	if err != nil {
		return failed(err)
	}

	r, err := b.resource(command.UUID, command.Resource)
	if err != nil {
		return failed(err)
	}

	outcome, err := b.strategy.Execute(ctx, r, command.Action, value)
	result.Output = tail(outcome.Output)

	// a failure says what it left the resource as only when its strategy
	// said so: a status it did not fill in says nothing.
	if err == nil || len(S(&outcome.Status).Common().State) > 0 {
		status, encodeErr := json.Marshal(outcome.Status)
		if encodeErr != nil {
			return failed(fmt.Errorf("the %s's status cannot be written: %w", b.descriptor.Name, encodeErr))
		}

		result.Status = status
	}

	if err != nil {
		return failed(err)
	}

	result.OK = true

	return result
}

func (b *nodeBinding[Spec, Status, S]) Query(ctx context.Context, query Query) (json.RawMessage, error) {
	a, err := actionOf(b.descriptor, query.Kind, query.Action, OnNode, ModeQuery)
	if err != nil {
		return nil, err
	}

	value, err := decodePayload(a, query.Payload)
	if err != nil {
		return nil, err
	}

	r, err := b.resource(query.UUID, query.Resource)
	if err != nil {
		return nil, err
	}

	if query.Action == "state" {
		return b.stateOf(ctx, &r)
	}

	answer, err := b.strategy.Query(ctx, r, query.Action, value)
	if err != nil {
		return nil, err
	}

	return json.Marshal(answer)
}

// stateOf is one resource's observation, read off everything the node holds
// of its kind, so that asking about one is answered the way the heartbeat
// reports them all.
func (b *nodeBinding[Spec, Status, S]) stateOf(ctx context.Context, r *Resource[Spec, Status]) (json.RawMessage, error) {
	report, err := b.State(ctx)
	if err != nil {
		return nil, err
	}

	if observed, listed := report.Find(r.Metadata.UUID); listed {
		return json.Marshal(observed)
	}

	var parent Reference
	if len(b.descriptor.Parent) > 0 {
		parent, _ = r.Metadata.Owner(b.descriptor.Parent)
	}

	switch {
	case report.Missing(r.Metadata.UUID, parent.UUID):
	case report.Unread(parent.UUID):
		return nil, fmt.Errorf("%w: the %s %q is inside %s %q, which is not running here", ErrUnreachable, b.descriptor.Name, r.Metadata.UUID, parent.Kind, parent.UUID)
	default:
		return nil, fmt.Errorf("%w: the %s %q is inside %s %q, which did not answer", ErrUnseen, b.descriptor.Name, r.Metadata.UUID, parent.Kind, parent.UUID)
	}

	var missing Status
	S(&missing).Common().State = Missing

	status, err := json.Marshal(missing)
	if err != nil {
		return nil, err
	}

	return json.Marshal(Observation{Kind: b.descriptor.Name, UUID: r.Metadata.UUID, Status: status})
}

func (b *nodeBinding[Spec, Status, S]) State(ctx context.Context) (Report[json.RawMessage], error) {
	typed, err := b.strategy.State(ctx)
	if err != nil {
		return Report[json.RawMessage]{}, err
	}

	report := Report[json.RawMessage]{
		Instances: make([]Observation, 0, len(typed.Instances)),
		Read:      slices.Clone(typed.Read),
		Unseen:    slices.Clone(typed.Unseen),
	}

	for _, instance := range typed.Instances {
		status, err := json.Marshal(instance.Status)
		if err != nil {
			return Report[json.RawMessage]{}, fmt.Errorf("the status of %s %q cannot be written: %w", b.descriptor.Name, instance.UUID, err)
		}

		report.Instances = append(report.Instances, Observation{
			Kind:   b.descriptor.Name,
			UUID:   instance.UUID,
			Owners: slices.Clone(instance.Owners),
			Status: status,
		})
	}

	return report, nil
}

func (b *nodeBinding[Spec, Status, S]) Attaches() bool {
	_, attaches := b.strategy.(Attacher)

	return attaches
}

func (b *nodeBinding[Spec, Status, S]) Attach(ctx context.Context, action string, uuid string, owner string) (Session, error) {
	if _, err := actionOf(b.descriptor, b.descriptor.Name, action, OnNode, ModeStream); err != nil {
		return nil, err
	}

	attacher, attaches := b.strategy.(Attacher)
	if !attaches {
		return nil, fmt.Errorf("%w: the %s's node strategy serves no streams", ErrUnknownAction, b.descriptor.Name)
	}

	return attacher.Attach(ctx, action, uuid, owner)
}

func (b *nodeBinding[Spec, Status, S]) Prompt() time.Duration {
	if prompt, prompts := b.strategy.(Prompt); prompts {
		return prompt.Prompt()
	}

	return 0
}

func (b *nodeBinding[Spec, Status, S]) Exposes() bool {
	_, exposes := b.strategy.(Exposer)

	return exposes
}

func (b *nodeBinding[Spec, Status, S]) Endpoint(ctx context.Context, slug string, p port.Port) (Endpoint, error) {
	exposer, exposes := b.strategy.(Exposer)
	if !exposes {
		return Endpoint{}, fmt.Errorf("%w: the %s's node strategy serves no ports", domain.ErrNotExists, b.descriptor.Name)
	}

	return exposer.Endpoint(ctx, slug, p)
}

// resource is the resource a command or a query carries, read as the kind's
// own, which has to be the one the command or the query names.
func (b *nodeBinding[Spec, Status, S]) resource(uuid string, raw Raw) (Resource[Spec, Status], error) {
	r, err := typedAs[Spec, Status](b.descriptor, raw)
	if errors.Is(err, ErrUnknownKind) {
		return Resource[Spec, Status]{}, err
	} else if err != nil {
		return Resource[Spec, Status]{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}

	if r.Metadata.UUID != uuid {
		return Resource[Spec, Status]{}, fmt.Errorf("%w: it names %s %q and carries %q", ErrInvalidPayload, b.descriptor.Name, uuid, r.Metadata.UUID)
	}

	return r, nil
}

type ingressBinding struct {
	Ingress

	descriptor Descriptor
}

var _ IngressBinding = ingressBinding{}

func (b ingressBinding) Descriptor() Descriptor {
	return b.descriptor
}

// typedAs reads a raw resource as d's kind's own types, refusing one of
// another kind.
func typedAs[Spec, Status any](d Descriptor, raw Raw) (Resource[Spec, Status], error) {
	if raw.Kind != d.Name {
		return Resource[Spec, Status]{}, fmt.Errorf("%w: a %q is not a %s", ErrUnknownKind, raw.Kind, d.Name)
	}

	return Decode[Spec, Status](raw)
}

// actionOf is the action of d a command, a query or a stream asks for, which
// has to be of d's kind and run where it is asked, the way it is asked.
func actionOf(d Descriptor, kindName string, name string, runs Executor, mode Mode) (Action, error) {
	if kindName != d.Name {
		return Action{}, fmt.Errorf("%w: a %q is not a %s", ErrUnknownKind, kindName, d.Name)
	}

	a, found := d.Action(name)
	if !found {
		return Action{}, fmt.Errorf("%w: a %s has no %q", ErrUnknownAction, d.Name, name)
	}

	if a.Runs != runs || a.Mode != mode {
		return Action{}, fmt.Errorf("%w: a %s's %q is a %s that runs on the %s, not a %s that runs on the %s", ErrUnknownAction, d.Name, name, a.Mode, a.Runs, mode, runs)
	}

	return a, nil
}

// decodePayload is what a payload decodes to with its action's codec, and an
// InvalidError for one that is not valid.
func decodePayload(a Action, raw []byte) (any, error) {
	value, invalid, err := a.Payload.Decode(raw)
	if err != nil {
		return nil, err
	}

	if len(invalid) > 0 {
		return nil, &InvalidError{Errors: invalid}
	}

	return value, nil
}
