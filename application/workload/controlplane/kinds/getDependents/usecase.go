// Package getDependents says which roots need a resource, as its delete would
// be answered with them now: the roots whose graphs need it, and whether one
// of them makes it again once it is gone, worked out as a delete works them
// out (dependents.Dependents.Of).
//
// It is a question and nothing else: nothing is changed, marked, queued or
// sent, and no node is asked anything. A resource is found as its delete
// finds it, inside the parent the request names, by whatever names it there;
// what nobody keeps a record of is needed by nothing, as its delete is
// answered with nothing that needs it; and what is not there, or not the
// owner's, is not there.
package getDependents

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository

	// needs are what a delete works out which roots need what it deletes
	// with.
	needs *dependents.Dependents
}

// NewUseCase says which roots need the resources kept in resources, as needs
// works it out, as a delete does.
func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository, needs *dependents.Dependents) *UseCase {
	return &UseCase{registry: registry, resources: resources, needs: needs}
}

// Execute is which roots need the resource request names. A kind that is not
// run here, and a resource that is not there or not the owner's, are errors:
// kind.ErrUnknownKind and domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	r, uuid, err := named.Resource(ctx, uc.resources, binding, request.OwnerUUID, request.Parent, request.UUID)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		// what nobody keeps a record of is needed by nothing, as its delete
		// says, once whoever asks may see it.
		if _, _, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, err); err != nil {
			return nil, err
		}

		return &Response{}, nil
	case err != nil:
		return nil, err
	}

	needs, err := uc.needs.Of(ctx, binding.Descriptor(), r)
	if err != nil {
		return nil, err
	}

	return &Response{Dependents: needs, Recreated: dependents.Recreated(needs)}, nil
}
