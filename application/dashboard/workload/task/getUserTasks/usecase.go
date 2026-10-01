package getusertasks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase lists the tasks one person asked the workload for.
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

	page, err := uc.workload.Tasks(ctx, request.OwnerUUID, request.Page)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, request.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items: presenter.NewTasks(page.Items, uc.ingressDomain, people),
		Pagination: presenter.Pagination{
			TotalPages:  page.TotalPages,
			CurrentPage: page.CurrentPage,
		},
	}, nil
}
