package getusertasks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase lists the tasks one person asked the runner for.
type UseCase struct {
	runner        runnerManager.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.runner.Tasks(ctx, request.OwnerUUID, request.Page)
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
