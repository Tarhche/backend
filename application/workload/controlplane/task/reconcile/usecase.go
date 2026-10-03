// Package reconcile brings the tasks back to what was asked of them.
//
// The workload is told what should be running, and the nodes report what is: a
// task stopped by hand, one whose process died, one removed from under the
// node, all leave the two disagreeing. This is the control plane's own heartbeat —
// it looks at that disagreement, over and over, and asks the node holding each
// task for the one thing that would close it.
//
// It says nothing about tasks on their way somewhere: something has been
// asked of those already, and asking again would only ask twice. Nor, for a
// while, about tasks their node cannot see because the class running them is
// out: those are unknown rather than gone.
package reconcile

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
)

const (
	// batch is how many tasks are read from the store at a time. A pass
	// works through every one of them, a batch at a time, rather than taking
	// the newest few: a task nobody looks at is a task nobody brings
	// back.
	batch uint = 20

	// silentAfter is how long a task may go unspoken for before what it
	// was last seen doing stops being believed. The nodes speak for theirs
	// several times a second, so this is many missed reports rather than one.
	silentAfter = 15 * time.Second
)

// UseCase is one pass over the tasks the workload holds.
type UseCase struct {
	taskRepository  task.Repository
	nodeRepository  node.Repository
	scheduler       *schedule.Scheduler
	asyncCommandBus domain.Producer

	// outageGrace is how long a task whose node says the class running it is
	// out is taken for unknown rather than silent
	// (WORKLOAD_RUNTIME_OUTAGE_GRACE). Zero takes none for unknown.
	outageGrace time.Duration

	logger *slog.Logger
}

func NewUseCase(
	taskRepository task.Repository,
	nodeRepository node.Repository,
	scheduler *schedule.Scheduler,
	asyncCommandBus domain.Producer,
	outageGrace time.Duration,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		nodeRepository:  nodeRepository,
		scheduler:       scheduler,
		asyncCommandBus: asyncCommandBus,
		outageGrace:     outageGrace,
		logger:          logger,
	}
}

// Execute looks at every task and asks for what is missing.
//
// It reads them a batch at a time rather than all at once, so that how many
// the workload is holding decides how long a pass takes rather than whether it
// covers them. What is being counted moves while it is being read — a
// task asked for during a pass shifts the rest along — so one may be
// looked at twice, which asks for what it needs twice and is the same answer,
// or missed, which the next pass ten seconds later picks up.
func (uc *UseCase) Execute(ctx context.Context) error {
	count, err := uc.taskRepository.Count(ctx)
	if err != nil {
		return err
	}

	// what they are judged against is when the pass began, so that a task
	// is not called silent for the time a long pass took to reach it.
	now := time.Now()

	// the nodes holding silent tasks, each read once a pass however many
	// of its tasks are silent.
	holders := newNodes(uc.nodeRepository)

	for offset := uint(0); offset < count; offset += batch {
		tasks, err := uc.taskRepository.GetAll(ctx, offset, batch)
		if err != nil {
			return err
		}

		// fewer are there than were counted: something was taken away while
		// this pass was reading them.
		if len(tasks) == 0 {
			return nil
		}

		for i := range tasks {
			uc.look(ctx, &tasks[i], now, holders)
		}
	}

	return nil
}

// look asks for what one task is missing, if it is missing anything.
func (uc *UseCase) look(ctx context.Context, t *task.Task, now time.Time, holders *nodes) {
	if !t.Drifted(now, silentAfter) {
		return
	}

	// a task that has ended while its node is still speaking for it
	// belongs to the failure chain: that is what counts the attempts at it
	// and decides whether there is another one. One that ended and then
	// went quiet is nobody's any more, and asking for it again is what
	// brings it back.
	if task.IsTerminalState(t.CurrentState) && !t.Silent(now, silentAfter) {
		return
	}

	if t.Silent(now, silentAfter) {
		unknown, err := uc.outage(ctx, t, now, holders)
		if err != nil {
			// whether it is gone cannot be told, and the next pass asks
			// again: that is ten seconds, against a duplicate of it.
			uc.logger.ErrorContext(ctx, "could not tell whether a silent task is gone", "error", err, "uuid", t.UUID, "node", t.NodeName)

			return
		}

		if unknown {
			uc.logger.InfoContext(ctx, "a task is not heard from while its node says its class is out; it is left as it is",
				"uuid", t.UUID, "name", t.Name, "node", t.NodeName, "runtime", t.Runtime.OrSysbox().String())

			return
		}
	}

	if err := uc.close(ctx, t); err != nil {
		// one task that cannot be dealt with is not a reason to leave
		// the rest as they are; the next pass tries it again.
		uc.logger.ErrorContext(ctx, "could not bring a task back to what was asked of it",
			"error", err, "uuid", t.UUID, "expected", t.ExpectedState.String(), "current", t.CurrentState.String())
	}
}

// close asks for the one thing that would put this task where it belongs.
func (uc *UseCase) close(ctx context.Context, t *task.Task) error {
	uc.logger.InfoContext(ctx, "a task is not what it was asked to be",
		"uuid", t.UUID, "name", t.Name, "expected", t.ExpectedState.String(), "current", t.CurrentState.String())

	switch t.ExpectedState {
	case task.Running:
		// one that was never placed anywhere has no node to ask: there was
		// nowhere to put it when it was asked for, so where it goes has to be
		// chosen again.
		if len(t.NodeName) == 0 {
			return uc.placeAgain(ctx, t)
		}

		// scheduling it again is what covers both a task that is merely
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

// scheduleAgain asks for a task that has drifted, as a first attempt: it
// is not a task that failed, but one that was taken away or stopped from
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

// placeAgain asks for a task to be placed, which is what was asked for
// when it was created and did not happen: no node was in a state to take it.
func (uc *UseCase) placeAgain(ctx context.Context, t *task.Task) error {
	payload, err := json.Marshal(events.TaskCreated{UUID: t.UUID})
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskCreatedName, payload)
}

// settle writes down that a task which is no longer anywhere has reached
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
