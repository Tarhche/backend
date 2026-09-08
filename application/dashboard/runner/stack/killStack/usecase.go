package killStack

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	UUID string `json:"-"`
}

// UseCase stops every service of a stack, at once and without a grace period. The runner owns its lifecycle, so this passes the
// command on rather than deciding anything about it.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.runner.KillStack(ctx, request.UUID)
}
