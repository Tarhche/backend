package killTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// UseCase stops a container at once, without the grace period a stop gives it.
// It travels the same states as a stop, because that is what it is: the
// difference is only how patient the worker is with the container.
type UseCase struct {
	taskRepository  task.Repository
	asyncCommandBus domain.Producer
	translator      translator.Translator
}

func NewUseCase(
	taskRepository task.Repository,
	asyncCommandBus domain.Producer,
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

	destinationState := task.Stopping
	if !task.ValidStateTransition(t.State, destinationState) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"task_id": uc.translator.Translate("invalid_state_transition"),
			},
		}, nil
	}

	t.State = destinationState
	if _, err = uc.taskRepository.Save(ctx, &t); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(events.TaskKillRequested{UUID: request.UUID})
	if err != nil {
		return nil, err
	}

	return nil, uc.asyncCommandBus.Produce(ctx, events.TaskKillRequestedName, payload)
}
