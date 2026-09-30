package getTasks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerControlPlane "github.com/khanzadimahdi/testproject/domain/runner/controlplane"
)

// UseCase lists the tasks the runner is holding.
type UseCase struct {
	runner        runnerControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(runner runnerControlPlane.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.runner.Tasks(ctx, "", request.Page)
	if err != nil {
		return nil, err
	}

	ownerUUIDs := make([]string, len(page.Items))
	for i := range page.Items {
		ownerUUIDs[i] = page.Items[i].OwnerUUID
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
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
