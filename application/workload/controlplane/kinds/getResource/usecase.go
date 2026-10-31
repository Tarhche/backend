// Package getResource reads one resource of any kind, as its manifest: one
// of the kind's records, or one of its extras, each to whoever may see it.
package getResource

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/named"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository) *UseCase {
	return &UseCase{registry: registry, resources: resources}
}

// Execute is the resource, or domain.ErrNotExists for one that is not there,
// not the owner's or not in the parent named, and kind.ErrUnknownKind for a
// kind not run here. An extra is whose its metadata says, as a record is: the
// code runner's runs are the guest's, there only to whoever may see anybody's.
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

	return &Response{Resource: r.Raw}, nil
}

// extra is the kind's extra the uuid names, when whoever asks may see it.
// notThere is what looking for a record came to, which is the answer
// otherwise.
func extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, uuid string, notThere error) (*Response, error) {
	_, r, err := named.Extra(ctx, binding, request.OwnerUUID, request.Parent, uuid, notThere)
	if err != nil {
		return nil, err
	}

	return &Response{Resource: r}, nil
}
