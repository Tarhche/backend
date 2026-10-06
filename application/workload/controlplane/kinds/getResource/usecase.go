// Package getResource reads one resource of any kind, as its manifest.
package getResource

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
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
// or not the owner's, and kind.ErrUnknownKind for a kind not run here.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	r, err := owner.Resource(ctx, uc.resources, binding.Descriptor().Name, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	return &Response{Resource: r.Raw}, nil
}
