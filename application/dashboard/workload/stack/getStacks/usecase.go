package getStacks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists the stacks the workload is holding.
type UseCase struct {
	workload      workloadControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{workload: workload, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.workload.Stacks(ctx, "", request.Page)
	if err != nil {
		return nil, err
	}

	// a stack and its services can belong to different people, so both are
	// asked about together.
	ownerUUIDs := make([]string, 0, len(page.Items))
	for i := range page.Items {
		ownerUUIDs = append(ownerUUIDs, page.Items[i].OwnerUUID)
		for j := range page.Items[i].Services {
			ownerUUIDs = append(ownerUUIDs, page.Items[i].Services[j].OwnerUUID)
		}
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items: presenter.NewStacks(page.Items, uc.ingressDomain, people),
		Pagination: presenter.Pagination{
			TotalPages:  page.TotalPages,
			CurrentPage: page.CurrentPage,
		},
	}, nil
}
