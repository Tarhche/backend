package stopContainer

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase stops a container. The runner owns its lifecycle, so this passes the
// command on rather than deciding anything about it.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.runner.StopContainer(ctx, request.UUID)
}
