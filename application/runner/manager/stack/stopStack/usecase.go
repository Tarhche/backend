package stopStack

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/runner/manager/stack/internal/fanout"
	stopTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase stops every service of a stack. A stack has no state of its own, so
// stopping it is exactly stopping the containers in it.
type UseCase struct {
	stackRepository stack.Repository
	taskRepository  task.Repository
	stopTask        *stopTask.UseCase
	logger          *slog.Logger
}

func NewUseCase(
	stackRepository stack.Repository,
	taskRepository task.Repository,
	stopTaskUseCase *stopTask.UseCase,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		taskRepository:  taskRepository,
		stopTask:        stopTaskUseCase,
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
	s.ExpectedState = task.Stopped
	if _, err := uc.stackRepository.Save(ctx, &s); err != nil {
		return nil, err
	}

	services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
	if err != nil {
		return nil, err
	}

	return nil, fanout.Over(ctx, services, uc.logger, func(ctx context.Context, uuid string) error {
		_, err := uc.stopTask.Execute(ctx, &stopTask.Request{UUID: uuid})

		return err
	})
}
