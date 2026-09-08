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
	asyncCommandBus domain.ProduceConsumer
	translator      translator.Translator
}

func NewUseCase(
	taskRepository task.Repository,
	asyncCommandBus domain.ProduceConsumer,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		asyncCommandBus: asyncCommandBus,
		translator:      translator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	t, err := uc.taskRepository.GetOne(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	// what it is asked to be from now on. It is written down before anything
	// is asked of the node, and whatever happens to that request, so that a
	// container which ends up somewhere else is brought back here by the
	// runner's own heartbeat.
	t.ExpectedState = task.Stopped

	destinationState := task.Stopping
	if !task.ValidStateTransition(t.CurrentState, destinationState) {
		// it cannot go there from where it is — but what was wanted is kept.
		if _, err := uc.taskRepository.Save(ctx, &t); err != nil {
			return nil, err
		}

		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"task_id": uc.translator.Translate("invalid_state_transition"),
			},
		}, nil
	}

	t.CurrentState = destinationState

	if _, err = uc.taskRepository.Save(ctx, &t); err != nil {
		return nil, err
	}

	event := events.TaskStoppageRequested{
		UUID: request.UUID,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	if err = uc.asyncCommandBus.Produce(ctx, events.TaskStoppageRequestedName, payload); err != nil {
		return nil, err
	}

	return nil, nil
}
