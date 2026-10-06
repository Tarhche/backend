// Package getResource reads one resource of any kind, as its manifest: one
// of the kind's records, or, to whoever may see anybody's, one of its extras.
package getResource

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
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

// Execute is the resource, or domain.ErrNotExists for one that is not there
// or not the owner's, and kind.ErrUnknownKind for a kind not run here. An
// extra is nobody's own, and is there only to whoever may see anybody's.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	r, err := owner.Resource(ctx, uc.resources, binding.Descriptor().Name, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return extra(ctx, binding, request, err)
	} else if err != nil {
		return nil, err
	}

	return &Response{Resource: r.Raw}, nil
}

// extra is the kind's extra the uuid names, when whoever asks may see
// anybody's. notThere is what looking for a record came to, which is the
// answer otherwise.
func extra(ctx context.Context, binding kind.ControlPlaneBinding, request *Request, notThere error) (*Response, error) {
	extras, extended := binding.Extras()
	if !extended || len(request.OwnerUUID) > 0 {
		return nil, notThere
	}

	r, err := extras.One(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	return &Response{Resource: r}, nil
}
