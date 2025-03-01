package stopTask

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
	asyncCommandBus domain.PublishSubscriber
	translator      translator.Translator
}

func NewUseCase(
	taskRepository task.Repository,
	asyncCommandBus domain.PublishSubscriber,
	translator translator.Translator,
) *UseCase {
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

	destinationState := task.Stopping
	if !task.ValidStateTransition(t.State, destinationState) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"task_id": uc.translator.Translate("invalid_state_transition"),
			},
		}, nil
	}

	t.State = destinationState
	if _, err = uc.taskRepository.Save(&t); err != nil {
		return nil, err
	}

	event := events.TaskStoppageRequested{
		UUID: request.UUID,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	if err = uc.asyncCommandBus.Publish(context.Background(), events.TaskStoppageRequestedName, payload); err != nil {
		return nil, err
	}

	return nil, nil
}
