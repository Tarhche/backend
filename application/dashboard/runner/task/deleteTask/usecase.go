package deleteTask

import (
	"context"

	runnerControlPlane "github.com/khanzadimahdi/testproject/domain/runner/controlplane"
)

// UseCase takes away a task. The runner owns its lifecycle, so this passes the
// command on rather than deciding anything about it.
type UseCase struct {
	runner runnerControlPlane.Client
}

func NewUseCase(runner runnerControlPlane.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.runner.DeleteTask(ctx, request.UUID)
}
