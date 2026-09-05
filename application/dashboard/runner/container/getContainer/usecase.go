package getContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase reads one container.

type UseCase struct {
	runner        runnerManager.Client
	owners        *owners.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *owners.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	c, err := uc.runner.Container(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, c.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Container: presenter.NewContainer(c, uc.ingressDomain, people)}, nil
}
