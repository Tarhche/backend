package heartbeatTask

import (
	"context"
	"encoding/json"
	"errors"

	deletetask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	killtask "github.com/khanzadimahdi/testproject/application/runner/manager/task/killTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type Heartbeat struct {
	taskRepository task.Repository
	producer       domain.Producer

	// deleteTask takes away a task that has finished and was only ever meant
	// to run once, which is the whole of it: the container, its log, and the
	// task itself. killTask stops a job that has run for too long.
	deleteTask *deletetask.UseCase
	killTask   *killtask.UseCase
}

var _ domain.MessageHandler = &Heartbeat{}

func NewHeartbeatHandler(
	taskRepository task.Repository,
	producer domain.Producer,
	deleteTask *deletetask.UseCase,
	killTask *killtask.UseCase,
) *Heartbeat {
	return &Heartbeat{
		taskRepository: taskRepository,
		producer:       producer,
		deleteTask:     deleteTask,
		killTask:       killTask,
	}
}

func (h *Heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.Heartbeat

	err := json.Unmarshal(data, &heartbeat)
	if err != nil {
		return err
	}

	t, err := h.taskRepository.GetOne(ctx, heartbeat.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	// a job's whole log rides its heartbeat; a service's is streamed line by
	// line and kept in the log repository instead.
	if t.Kind != task.KindService {
		t.ContainerLogs = heartbeat.Logs
	}

	// what the node says it is doing, and what that means for a container that
	// was supposed to be doing something else.
	reportedState := task.State(heartbeat.State)
	taskState := reportedState

	// what it was doing before this report, so that what happened to it can be
	// told apart from what it has been doing all along, and what was wanted of
	// it before this report was allowed to change that.
	previousState := t.CurrentState
	wantedState := t.ExpectedState

	// a service is meant to keep running, so one that has ended without being
	// asked to has not stopped: it failed to stay up, and that is what it is
	// until somebody asks it for something else.
	if t.Kind == task.KindService && wantedState != task.Stopped && task.IsTerminalState(taskState) {
		taskState = task.Failed
	}

	// what the node holding it says it is doing, and that it said anything at
	// all: a container nobody speaks for is one that is no longer there.
	t.CurrentState = taskState
	t.LastHeartbeatAt = heartbeat.At

	// a container that is running was not given up on after all: it came back,
	// so keeping it up is what is wanted of it again.
	if taskState == task.Running && t.ExpectedState == task.Failed {
		t.ExpectedState = task.Running
	}

	// a job that has ended is not asked for anything further, so nothing tries
	// to bring it back.
	if t.Kind == task.KindJob && task.IsTerminalState(taskState) {
		t.ExpectedState = taskState
	}

	if _, err = h.taskRepository.Save(ctx, &t); err != nil {
		return err
	}

	switch {
	case taskState == task.Running:
		// a running container is reported over and over, and each report
		// carries the addresses its ports came up on, which change under it.
		err = h.publishTaskRan(ctx, &heartbeat)

	case taskState == task.Failed:
		// a container that has failed says so in every report it makes until
		// somebody takes it away. The moment it failed is announced once, and
		// after that the repeats are what ask for the next attempt, when the
		// wait between attempts is over.
		if previousState != task.Failed || t.RetryDue(heartbeat.At, heartbeat.Attempt) {
			err = h.publishTaskFailed(ctx, &heartbeat, &t, failureReason(reportedState))
		}

	case previousState == taskState:
		// it is where it already was. What happened to it was announced when
		// it happened, and saying so again would only have it answered twice.

	case taskState == task.Stopped:
		err = h.publishTaskStopped(ctx, &heartbeat)

	case taskState == task.Completed:
		err = h.publishTaskCompleted(ctx, &heartbeat)
	}

	if err != nil {
		return err
	}

	if t.Removable() {
		return h.remove(ctx, heartbeat.UUID)
	}

	// a job that has run for longer than it asked for is stopped here rather
	// than on the node holding it: a timer on a worker dies with the worker,
	// while what a job was allowed is written down and outlives both.
	if t.OutlivedTTL(heartbeat.At) {
		return h.kill(ctx, heartbeat.UUID)
	}

	return nil
}

// failureReason says what went wrong in the words the dashboard shows: what
// the node reported is what tells a container that fell over apart from one
// that ended when nobody asked it to.
func failureReason(reported task.State) string {
	if reported == task.Failed {
		return "the container failed"
	}

	return "the container stopped without being asked to"
}

// kill stops a job that has outlived its ttl.
//
// It is asked for the same way the dashboard asks for it, which is also what
// keeps a job from being killed over and over while it is on its way out: a
// task already stopping is not a task that can be told to stop.
func (h *Heartbeat) kill(ctx context.Context, uuid string) error {
	_, err := h.killTask.Execute(ctx, &killtask.Request{UUID: uuid})

	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

// remove takes away a task that was only ever meant to run once. A container
// the code runner started has said everything it is going to say by the time it
// finishes, so what is left of it — the container, its log and the task itself
// — goes with it rather than staying in every listing forever.
//
// Forced, because the container has just reported that it finished: waiting for
// the stored state to catch up with what this heartbeat already says would
// leave the task behind on the first report and take it away on some later one.
func (h *Heartbeat) remove(ctx context.Context, uuid string) error {
	_, err := h.deleteTask.Execute(ctx, &deletetask.Request{UUID: uuid, Force: true})

	// a heartbeat for a task already taken away, which is the outcome this was
	// after anyway.
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return err
}

func (uc *Heartbeat) publishTaskRan(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskRan{
		UUID:          heartbeat.UUID,
		NodeName:      heartbeat.NodeName,
		ContainerUUID: heartbeat.ContainerUUID,
		Endpoints:     heartbeat.Endpoints,
		StartedAt:     heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskRanName, payload)
}

func (uc *Heartbeat) publishTaskStopped(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskStopped{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskStoppedName, payload)
}

func (uc *Heartbeat) publishTaskCompleted(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskCompleted{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskCompletedName, payload)
}

func (uc *Heartbeat) publishTaskFailed(ctx context.Context, heartbeat *events.Heartbeat, t *task.Task, reason string) error {
	event := events.TaskFailed{
		UUID:          heartbeat.UUID,
		ContainerUUID: heartbeat.ContainerUUID,
		NodeName:      heartbeat.NodeName,
		At:            heartbeat.At,
		Attempt:       heartbeat.Attempt,
		MaxRetries:    t.MaxRetries,
		Reason:        reason,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskFailedName, payload)
}
