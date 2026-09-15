package deleteusertask

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase takes away one of somebody's own tasks.
//
// The task is read as theirs first, so one that is somebody else's is not
// found rather than refused, and nothing is asked of the runner about it.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if _, err := uc.runner.TaskOf(ctx, request.OwnerUUID, request.UUID); err != nil {
		return err
	}

	return uc.runner.DeleteTask(ctx, request.UUID)
}
