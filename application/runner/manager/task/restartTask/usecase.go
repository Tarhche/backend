package restartTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// UseCase stops a container and starts it again in place. The container keeps
// its identity, so its slug, its log and its place in a stack all survive; only
// the host ports it is published on may change.
type UseCase struct {
	taskRepository  task.Repository
	scheduler       *schedule.Scheduler
	asyncCommandBus domain.Producer
	translator      translator.Translator
}

func NewUseCase(
	taskRepository task.Repository,
	scheduler *schedule.Scheduler,
	asyncCommandBus domain.Producer,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		scheduler:       scheduler,
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
	t.ExpectedState = task.Running

	// a container that has ended is not restarted where it stands: there may be
	// nothing left to restart, and somebody asking for it by hand is asking for
	// a fresh start rather than one more of the attempts behind it.
	if task.IsTerminalState(t.CurrentState) {
		t.CurrentState = task.Scheduled
		t.Retries = 0

		if _, err := uc.taskRepository.Save(ctx, &t); err != nil {
			return nil, err
		}

		return nil, uc.scheduler.On(ctx, &t, t.NodeName, 0)
	}

	destinationState := task.Restarting
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

	payload, err := json.Marshal(events.TaskRestartRequested{UUID: request.UUID})
	if err != nil {
		return nil, err
	}

	return nil, uc.asyncCommandBus.Produce(ctx, events.TaskRestartRequestedName, payload)
}
