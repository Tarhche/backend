// Package admitResource takes in a resource of any kind that somebody asks
// for: its kind admits it, it is kept, and it is sent the first command its
// kind asks for.
//
// What a resource is admitted as is its kind's to say, through its
// control-plane strategy: its defaults, what is wrong with it, its owner's
// quotas and the node it is placed on. What is the same for every kind is
// said here: whom it belongs to, when it was made, when its lifetime ends,
// and the state its machine starts it in.
//
// Its first command is what the kind's Reconcile asks for of it as it was
// admitted: a VM to be made, say. It is sent as soon as the resource is
// kept, rather than on the reconcile loop's next pass, and the reconcile loop
// sends it again if it is lost.
package admitResource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// attempts is how many times a resource is admitted before giving up, when
// another took the slug it was given in the meantime: admitted again, it is
// given another.
const attempts = 3

type UseCase struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	dispatcher *dispatch.Dispatcher
	logger     *slog.Logger
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, dispatcher *dispatch.Dispatcher, logger *slog.Logger) *UseCase {
	return &UseCase{registry: registry, resources: resources, dispatcher: dispatcher, logger: logger}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()

	if invalid := validate(d, request); len(invalid) > 0 {
		return &Response{ValidationErrors: invalid}, nil
	}

	asked := askedFor(d, request)

	var created resource.Record

	for try := 1; ; try++ {
		admitted, invalid, err := binding.Admit(ctx, asked)
		switch {
		case errors.Is(err, kind.ErrInvalidPayload):
			return &Response{ValidationErrors: domain.ValidationErrors{"spec": "invalid_value"}}, nil
		case err != nil:
			return nil, err
		case len(invalid) > 0:
			return &Response{ValidationErrors: invalid}, nil
		}

		record, err := uc.kept(d, request.OwnerUUID, admitted)
		if err != nil {
			return nil, err
		}

		created, err = uc.resources.Create(ctx, record)
		if errors.Is(err, domain.ErrAlreadyExists) && try < attempts {
			continue
		} else if err != nil {
			return nil, err
		}

		break
	}

	response := &Response{Resource: created.Raw}

	// what was asked of it before it fell short is written down, and what is
	// left is the reconcile loop's to ask for.
	dispatched, err := uc.dispatcher.Follow(ctx, binding, created)
	if err != nil {
		uc.logger.ErrorContext(ctx, "could not ask a resource just admitted for what its kind asks", "error", err, "kind", d.Name, "uuid", created.Metadata.UUID)
	}

	if dispatched.Gone {
		return response, nil
	}

	response.Resource = dispatched.Record.Raw

	if dispatched.Command == nil {
		return response, nil
	}

	response.Command = dispatched.Command

	result, err := uc.dispatcher.Send(ctx, *dispatched.Command, request.Wait)
	if err != nil {
		// written down as what it waits on, it is sent again by the
		// reconcile loop: the resource is kept either way.
		uc.logger.ErrorContext(ctx, "could not send a resource its first command", "error", err, "kind", d.Name, "uuid", created.Metadata.UUID, "action", dispatched.Command.Action)

		return response, nil
	}

	if result == nil {
		return response, nil
	}

	response.Result = result

	if latest, err := uc.resources.GetOne(ctx, d.Name, created.Metadata.UUID); err == nil {
		response.Resource = latest.Raw
	}

	return response, nil
}

// kept is a resource as its kind admitted it, as it is kept: for whom it was
// asked, made now, with the end of its lifetime, in the state its machine
// starts it in unless its kind said otherwise.
func (uc *UseCase) kept(d kind.Descriptor, ownerUUID string, admitted kind.Raw) (resource.Record, error) {
	now := uc.dispatcher.Now()

	r := resource.Record{Raw: admitted}
	r.Kind = d.Name
	r.Metadata.OwnerUUID = ownerUUID
	r.Metadata.CreatedAt = now
	r.Metadata.UpdatedAt = now

	if r.Metadata.Lifetime > 0 && r.Metadata.ExpiresAt.IsZero() {
		r.Metadata.ExpiresAt = now.Add(r.Metadata.Lifetime)
	}

	common, err := r.Common()
	if err != nil {
		return resource.Record{}, err
	}

	if len(common.State) == 0 {
		common.State = d.Machine.Initial
	}

	if !d.Machine.Has(common.State) || common.State == kind.Deleted {
		return resource.Record{}, fmt.Errorf("a %s cannot be admitted %q, which is not a state it can start in", d.Name, common.State)
	}

	common.Since = now
	common.ObservedAt = time.Time{}

	if err := r.SetCommon(common); err != nil {
		return resource.Record{}, err
	}

	return r, nil
}

// validate is what is wrong with a request whatever the kind.
func validate(d kind.Descriptor, request *Request) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	if len(request.OwnerUUID) == 0 {
		invalid["owner"] = "required_field"
	}

	if len(request.Manifest.Kind) > 0 && request.Manifest.Kind != d.Name {
		invalid["kind"] = "invalid_value"
	}

	if len(request.Parent) > 0 && len(d.Parent) == 0 {
		invalid["parent"] = "invalid_value"
	}

	if request.Manifest.Metadata.Lifetime < 0 {
		invalid["lifetime"] = "invalid_value"
	}

	return invalid
}

// askedFor is what the kind is asked to admit: what the caller may say of a
// resource, and nothing else.
func askedFor(d kind.Descriptor, request *Request) kind.Raw {
	m := request.Manifest.Metadata

	asked := kind.Raw{
		Kind: d.Name,
		Metadata: kind.Metadata{
			Name:      m.Name,
			OwnerUUID: request.OwnerUUID,
			Labels:    m.Labels,
			Owners:    slices.Clone(m.Owners),
			Lifetime:  m.Lifetime,
		},
		Spec: request.Manifest.Spec,
	}

	if len(request.Parent) > 0 {
		parent := kind.Reference{Kind: d.Parent, UUID: request.Parent}

		if !slices.Contains(asked.Metadata.Owners, parent) {
			asked.Metadata.Owners = append(asked.Metadata.Owners, parent)
		}
	}

	return asked
}
