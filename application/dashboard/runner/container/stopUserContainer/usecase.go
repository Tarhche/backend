package stopusercontainer

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase stops one of somebody's own containers.
//
// The container is read as theirs first, so one that is somebody else's is not
// found rather than refused, and nothing is asked of the runner about it.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if _, err := uc.runner.ContainerOf(ctx, request.OwnerUUID, request.UUID); err != nil {
		return err
	}

	return uc.runner.StopContainer(ctx, request.UUID)
}
