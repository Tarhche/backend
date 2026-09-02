package getContainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	Page uint `json:"page"`
}

type Response struct {
	Items      []presenter.Container `json:"items"`
	Pagination presenter.Pagination  `json:"pagination"`
}

// UseCase lists the containers the runner is holding.
type UseCase struct {
	runner        runnerManager.Client
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	page, err := uc.runner.Containers(ctx, request.Page)
	if err != nil {
		return nil, err
	}

	return &Response{
		Items: presenter.NewContainers(page.Items, uc.ingressDomain),
		Pagination: presenter.Pagination{
			TotalPages:  page.TotalPages,
			CurrentPage: page.CurrentPage,
		},
	}, nil
}
