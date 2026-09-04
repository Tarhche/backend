package deletetask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	taskRepository  task.Repository
	logRepository   container.LogRepository
	asyncCommandBus domain.Producer
	translator      translator.Translator
}

func NewUseCase(
	taskRepository task.Repository,
	logRepository container.LogRepository,
	asyncCommandBus domain.Producer,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		logRepository:   logRepository,
		asyncCommandBus: asyncCommandBus,
		translator:      translator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	t, err := uc.taskRepository.GetOne(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	if !request.Force && !task.IsTerminalState(t.CurrentState) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"task_id": uc.translator.Translate("task_is_not_terminal_state"),
			},
		}, nil
	}

	if err := uc.publishTaskDeleted(ctx, request.UUID); err != nil {
		return nil, err
	}

	// the task goes first, because it is what the dashboard shows: a log that
	// could not be swept is worth reporting, but it is not a reason to keep
	// showing a container that is meant to be gone.
	if err := uc.taskRepository.Delete(ctx, request.UUID); err != nil {
		return nil, err
	}

	// a container's log lives exactly as long as the container does.
	return nil, uc.logRepository.DeleteByTask(ctx, request.UUID)
}

func (uc *UseCase) publishTaskDeleted(ctx context.Context, uuid string) error {
	event := events.TaskDeleted{
		UUID: uuid,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskDeletedName, payload)
}
