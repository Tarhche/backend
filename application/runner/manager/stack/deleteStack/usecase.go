package deleteStack

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/runner/manager/stack/internal/fanout"
	deleteTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase removes a stack and everything it owns: its containers, their logs,
// and the private network its services shared.
//
// A service still running goes down with it. Deleting a stack is a request to
// have it gone rather than to be told that something is still up.
type UseCase struct {
	stackRepository stack.Repository
	taskRepository  task.Repository
	deleteTask      *deleteTask.UseCase
	asyncCommandBus domain.Producer
	logger          *slog.Logger
}

func NewUseCase(
	stackRepository stack.Repository,
	taskRepository task.Repository,
	deleteTaskUseCase *deleteTask.UseCase,
	asyncCommandBus domain.Producer,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		taskRepository:  taskRepository,
		deleteTask:      deleteTaskUseCase,
		asyncCommandBus: asyncCommandBus,
		logger:          logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.stackRepository.GetOne(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
	if err != nil {
		return nil, err
	}

	if err := fanout.Over(ctx, services, uc.logger, func(ctx context.Context, uuid string) error {
		_, err := uc.deleteTask.Execute(ctx, &deleteTask.Request{UUID: uuid, Force: true})
		if errors.Is(err, domain.ErrNotExists) {
			return nil
		}

		return err
	}); err != nil {
		return nil, err
	}

	if err := uc.publishStackDeleted(ctx, &s); err != nil {
		return nil, err
	}

	return nil, uc.stackRepository.Delete(ctx, s.UUID)
}

// publishStackDeleted tells the node that ran the stack to drop the private
// network its services shared, now that nothing is left on it.
func (uc *UseCase) publishStackDeleted(ctx context.Context, s *stack.Stack) error {
	payload, err := json.Marshal(stackEvents.StackDeleted{
		UUID:     s.UUID,
		Slug:     s.Slug,
		NodeName: s.NodeName,
		At:       time.Now(),
	})
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, stackEvents.StackDeletedName, payload)
}
