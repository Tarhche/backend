package deleteStack

import (
	"context"

	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Request struct {
	UUID string `json:"-"`

	// ActorUUID is who is asking, which decides whether this one is theirs
	// to touch.
	ActorUUID string `json:"-"`
}

// UseCase removes a stack: its services, their logs and the private network they shared. The runner owns its lifecycle, so this passes the
// command on rather than deciding anything about it.
type UseCase struct {
	runner runnerManager.Client
	guard  *runnerAccess.Guard
}

func NewUseCase(runner runnerManager.Client, guard *runnerAccess.Guard) *UseCase {
	return &UseCase{runner: runner, guard: guard}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	if err := uc.guard.OverStack(ctx, request.ActorUUID, permission.RunnerStacksDelete, permission.SelfRunnerStacksDelete, request.UUID); err != nil {
		return err
	}

	return uc.runner.DeleteStack(ctx, request.UUID)
}
