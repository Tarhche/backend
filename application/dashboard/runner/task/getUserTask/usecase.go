package getusertask

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase reads one of somebody's own tasks. One that is somebody else's
// is not found rather than refused.
type UseCase struct {
	runner        runnerManager.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	c, err := uc.runner.TaskOf(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, c.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Task: presenter.NewTask(c, uc.ingressDomain, people)}, nil
}
