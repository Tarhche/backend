// Package reconcile brings the containers back to what was asked of them.
//
// The runner is told what should be running, and the nodes report what is: a
// container stopped by hand, one whose process died, one removed from under the
// node, all leave the two disagreeing. This is the manager's own heartbeat —
// it looks at that disagreement, over and over, and asks the node holding each
// container for the one thing that would close it.
//
// It says nothing about containers on their way somewhere: something has been
// asked of those already, and asking again would only ask twice.
package reconcile

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	// limit is how many containers one pass looks at.
	limit uint = 200

	// silentAfter is how long a container may go unspoken for before what it
	// was last seen doing stops being believed. The nodes speak for theirs
	// several times a second, so this is many missed reports rather than one.
	silentAfter = 15 * time.Second
)

// UseCase is one pass over the containers the runner holds.
type UseCase struct {
	taskRepository  task.Repository
	scheduler       *schedule.Scheduler
	asyncCommandBus domain.Producer
	logger          *slog.Logger
}

func NewUseCase(
	taskRepository task.Repository,
	scheduler *schedule.Scheduler,
	asyncCommandBus domain.Producer,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		scheduler:       scheduler,
		asyncCommandBus: asyncCommandBus,
		logger:          logger,
	}
}

// Execute looks at every container and asks for what is missing.
func (uc *UseCase) Execute(ctx context.Context) error {
	tasks, err := uc.taskRepository.GetAll(ctx, 0, limit)
	if err != nil {
		return err
	}

	now := time.Now()

	for i := range tasks {
		t := &tasks[i]

		if !t.Drifted(now, silentAfter) {
			continue
		}

		// a container that has ended while its node is still speaking for it
		// belongs to the failure chain: that is what counts the attempts at it
		// and decides whether there is another one. One that ended and then
		// went quiet is nobody's any more, and asking for it again is what
		// brings it back.
		if task.IsTerminalState(t.CurrentState) && !t.Silent(now, silentAfter) {
			continue
		}

		if err := uc.close(ctx, t); err != nil {
			// one container that cannot be dealt with is not a reason to leave
			// the rest as they are; the next pass tries it again.
			uc.logger.ErrorContext(ctx, "could not bring a container back to what was asked of it",
				"error", err, "uuid", t.UUID, "expected", t.ExpectedState.String(), "current", t.CurrentState.String())
		}
	}

	return nil
}

// close asks for the one thing that would put this container where it belongs.
func (uc *UseCase) close(ctx context.Context, t *task.Task) error {
	uc.logger.InfoContext(ctx, "a container is not what it was asked to be",
		"uuid", t.UUID, "name", t.Name, "expected", t.ExpectedState.String(), "current", t.CurrentState.String())

	switch t.ExpectedState {
	case task.Running:
		// one that was never placed anywhere has no node to ask: there was
		// nowhere to put it when it was asked for, so where it goes has to be
		// chosen again.
		if len(t.NodeName) == 0 {
			return uc.placeAgain(ctx, t)
		}

		// scheduling it again is what covers both a container that is merely
		// stopped and one that is no longer there: the node starts the one it
		// still has, and makes the one it does not.
		return uc.scheduleAgain(ctx, t)

	case task.Stopped:
		// nobody is holding it any more, so it is not running: what was asked
		// for is already true, and saying so is what ends the asking.
		if t.Silent(time.Now(), silentAfter) {
			return uc.settle(ctx, t)
		}

		return uc.stop(ctx, t)

	default:
		return nil
	}
}

// scheduleAgain asks for a container that has drifted, as a first attempt: it
// is not a container that failed, but one that was taken away or stopped from
// somewhere else, and there is nothing behind it to count.
func (uc *UseCase) scheduleAgain(ctx context.Context, t *task.Task) error {
	if t.Retries != 0 {
		t.Retries = 0

		if _, err := uc.taskRepository.Save(ctx, t); err != nil {
			return err
		}
	}

	return uc.scheduler.On(ctx, t, t.NodeName, 0)
}

// placeAgain asks for a container to be placed, which is what was asked for
// when it was created and did not happen: no node was in a state to take it.
func (uc *UseCase) placeAgain(ctx context.Context, t *task.Task) error {
	payload, err := json.Marshal(events.TaskCreated{UUID: t.UUID})
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskCreatedName, payload)
}

// settle writes down that a container which is no longer anywhere has reached
// what was asked of it, so that nothing keeps asking.
func (uc *UseCase) settle(ctx context.Context, t *task.Task) error {
	t.CurrentState = t.ExpectedState

	_, err := uc.taskRepository.Save(ctx, t)

	return err
}

func (uc *UseCase) stop(ctx context.Context, t *task.Task) error {
	payload, err := json.Marshal(events.TaskStoppageRequested{UUID: t.UUID})
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskStoppageRequestedName, payload)
}
