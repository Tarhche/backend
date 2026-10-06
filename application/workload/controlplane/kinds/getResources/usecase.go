// Package getResources lists the resources of any kind, a page at a time.
package getResources

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Limit is how many resources a page holds.
const Limit uint = 20

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository) *UseCase {
	return &UseCase{registry: registry, resources: resources}
}

// Execute is the page asked for, or kind.ErrUnknownKind for a kind not run
// here.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()
	offset, page := presenter.Offset(request.Page, Limit)

	filter := resource.Filter{OwnerUUID: request.OwnerUUID}
	if len(request.Parent) > 0 {
		filter.Parent = kind.Reference{Kind: d.Parent, UUID: request.Parent}
	}

	records, total, err := uc.resources.GetAll(ctx, d.Name, filter, offset, Limit)
	if err != nil {
		return nil, err
	}

	items := make([]kind.Raw, len(records))
	for i := range records {
		items[i] = records[i].Raw
	}

	return &Response{Items: items, Pagination: presenter.NewPagination(total, Limit, page)}, nil
}
