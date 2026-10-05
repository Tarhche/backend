package getStacks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists stacks: everybody's or one person's own, in every VM or in
// one. A listing leaves their compose files out.
type UseCase struct {
	workload workloadControlPlane.Client
	owners   *presenter.Directory
}

func NewUseCase(workload workloadControlPlane.Client, owners *presenter.Directory) *UseCase {
	return &UseCase{workload: workload, owners: owners}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.workload.Stacks(ctx, request.OwnerUUID, request.VMUUID, request.Page)
	if err != nil {
		return nil, err
	}

	ownerUUIDs := make([]string, len(page.Items))
	for i := range page.Items {
		ownerUUIDs[i] = page.Items[i].OwnerUUID
	}

	owners, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items:      presenter.NewStacks(page.Items, owners),
		Pagination: presenter.NewPagination(page),
	}, nil
}
