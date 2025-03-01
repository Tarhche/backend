package deletetask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	taskRepository  task.Repository
	asyncCommandBus domain.Publisher
	translator      translator.Translator
}

func NewUseCase(taskRepository task.Repository, asyncCommandBus domain.Publisher, translator translator.Translator) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		asyncCommandBus: asyncCommandBus,
		translator:      translator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	t, err := uc.taskRepository.GetOne(request.UUID)
	if err != nil {
		return nil, err
	}

	if !task.IsTerminalState(t.State) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"task_id": uc.translator.Translate("task_is_not_terminal_state"),
			},
		}, nil
	}

	if err := uc.publishTaskDeleted(request.UUID); err != nil {
		return nil, err
	}

	return nil, uc.taskRepository.Delete(request.UUID)
}

func (uc *UseCase) publishTaskDeleted(uuid string) error {
	event := events.TaskDeleted{
		UUID: uuid,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Publish(context.Background(), events.TaskDeletedName, payload)
}
