package getusercontainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	Page uint `json:"page"`

	// OwnerUUID is whose containers these are. It is not asked for: it is who is
	// asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}

type Response struct {
	Items      []presenter.Container `json:"items"`
	Pagination presenter.Pagination  `json:"pagination"`
}

// UseCase lists the containers one person asked the runner for.
type UseCase struct {
	runner        runnerManager.Client
	owners        *owners.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *owners.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.runner.Containers(ctx, request.OwnerUUID, request.Page)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, request.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items: presenter.NewContainers(page.Items, uc.ingressDomain, people),
		Pagination: presenter.Pagination{
			TotalPages:  page.TotalPages,
			CurrentPage: page.CurrentPage,
		},
	}, nil
}
