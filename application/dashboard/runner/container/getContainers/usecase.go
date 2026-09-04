package getContainers

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	Page uint `json:"page"`

	// OwnerUUID narrows the listing to the person asking. Empty is everybody's,
	// which is what somebody who may see all of them gets.
	OwnerUUID string `json:"-"`
}

type Response struct {
	Items      []presenter.Container `json:"items"`
	Pagination presenter.Pagination  `json:"pagination"`
}

// UseCase lists the containers the runner is holding.
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

	ownerUUIDs := make([]string, len(page.Items))
	for i := range page.Items {
		ownerUUIDs[i] = page.Items[i].OwnerUUID
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
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
