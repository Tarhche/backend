package restartStack

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/runner/manager/stack/internal/fanout"
	restartTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/restartTask"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase restarts every service of a stack. A stack has no state of its own,
// the command reaches the containers in it, one by one.
type UseCase struct {
	stackRepository stack.Repository
	taskRepository  task.Repository
	restartTask     *restartTask.UseCase
	logger          *slog.Logger
}

func NewUseCase(
	stackRepository stack.Repository,
	taskRepository task.Repository,
	restartTaskUseCase *restartTask.UseCase,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		taskRepository:  taskRepository,
		restartTask:     restartTaskUseCase,
		logger:          logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.stackRepository.GetOne(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	// what the stack is asked to be from now on, written down before anything
	// is asked of its services: while a command is still reaching them, what
	// they say between them is neither one thing nor the other.
	s.ExpectedState = task.Running
	if _, err := uc.stackRepository.Save(ctx, &s); err != nil {
		return nil, err
	}

	services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
	if err != nil {
		return nil, err
	}

	return nil, fanout.Over(ctx, services, uc.logger, func(ctx context.Context, uuid string) error {
		_, err := uc.restartTask.Execute(ctx, &restartTask.Request{UUID: uuid})

		return err
	})
}
