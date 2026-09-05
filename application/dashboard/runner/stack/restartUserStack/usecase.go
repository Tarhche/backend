package restartuserstack

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	UUID string `json:"-"`

	// OwnerUUID is whose stack this has to be. It is not asked for: it is who
	// is asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}

// UseCase restarts one of somebody's own stacks.
//
// The stack is read as theirs first, so one that is somebody else's is not
// found rather than refused, and nothing is asked of the runner about it.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if _, err := uc.runner.StackOf(ctx, request.OwnerUUID, request.UUID); err != nil {
		return err
	}

	return uc.runner.RestartStack(ctx, request.UUID)
}
