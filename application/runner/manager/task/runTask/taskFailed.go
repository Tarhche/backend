package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	deletetask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// TaskFailed is what becomes of a container that has failed.
//
// Every failure is written down — which attempt it was, and what went wrong —
// and then answered. A container still worth trying is asked for again, one
// attempt further along; one that has run out of attempts is left where it is,
// and what was expected of it is set to that, so nothing keeps asking for
// something that is not going to happen.
type TaskFailed struct {
	taskRepository task.Repository
	logRepository  container.LogRepository
	scheduler      *schedule.Scheduler

	// deleteTask takes away a job that has failed for good. A job that never
	// got a container never reports a heartbeat either, so this is the only
	// place that hears it is over.
	deleteTask *deletetask.UseCase

	logger *slog.Logger
}

var _ domain.MessageHandler = &TaskFailed{}

func NewTaskFailed(
	taskRepository task.Repository,
	logRepository container.LogRepository,
	scheduler *schedule.Scheduler,
	deleteTask *deletetask.UseCase,
	logger *slog.Logger,
) *TaskFailed {
	return &TaskFailed{
		taskRepository: taskRepository,
		logRepository:  logRepository,
		scheduler:      scheduler,
		deleteTask:     deleteTask,
		logger:         logger,
	}
}

func (uc *TaskFailed) Handle(ctx context.Context, data []byte) error {
	var taskFailed events.TaskFailed
	if err := json.Unmarshal(data, &taskFailed); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskFailed.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	// which attempt this failure belongs to is the node's count, unless the
	// container had been standing long enough that what came before it no
	// longer has anything to do with it.
	attempt := t.Attempt(taskFailed.Attempt, taskFailed.At)

	// whether there is another attempt coming is decided before the failure is
	// written down, because part of what is written down is that there is one.
	retrying := t.ExpectedState == task.Running && t.MayRetry(attempt)

	if err := uc.record(ctx, &t, &taskFailed, attempt, retrying); err != nil {
		return err
	}

	// only a container that is still wanted running is worth another attempt:
	// one that failed on its way to being stopped has arrived.
	if retrying {
		// and only when it has been left alone long enough. The node holding
		// it goes on reporting it, and one of those reports comes back here
		// when the wait is over.
		if !t.RetryDue(time.Now(), attempt) {
			return nil
		}

		return uc.again(ctx, &t, attempt+1)
	}

	return uc.giveUp(ctx, &t)
}

// record writes down that an attempt failed: on the task, where the dashboard
// reads it, and against the container's log, where somebody looking into a
// container that keeps dying will find every attempt rather than the last.
func (uc *TaskFailed) record(ctx context.Context, t *task.Task, failure *events.TaskFailed, attempt int, retrying bool) error {
	uc.logger.WarnContext(ctx, "a container failed",
		"uuid", t.UUID, "name", t.Name, "node", failure.NodeName,
		"attempt", attempt, "max_retries", t.MaxRetries, "reason", failure.Reason)

	reason := uc.reason(t, failure, attempt)

	// the same failure reported again is the node asking whether it is time to
	// try once more, rather than something else having gone wrong. What is
	// written down about it — when it happened above all — stands.
	repeated := t.CurrentState == task.Failed && t.Reason == reason

	at := failure.At
	if at.IsZero() {
		at = time.Now()
	}

	t.CurrentState = task.Failed
	t.Reason = reason

	// the attempt that is coming, so that a container which has failed says
	// what is being done about it rather than what has been done so far.
	if retrying {
		t.Retries = attempt + 1
	}

	if !repeated {
		t.FinishedAt = at
	}

	if _, err := uc.taskRepository.Save(ctx, t); err != nil {
		return err
	}

	if repeated {
		return nil
	}

	return uc.logRepository.Append(ctx, []container.Log{{
		TaskUUID:    t.UUID,
		ContainerID: failure.ContainerUUID,
		LogLine: container.LogLine{
			Stream:  container.StreamStderr,
			Content: t.Reason,
			At:      at,
		},
	}})
}

// reason says what happened in the words the dashboard shows: what went wrong,
// and where that leaves the container.
func (uc *TaskFailed) reason(t *task.Task, failure *events.TaskFailed, attempt int) string {
	cause := failure.Reason
	if len(cause) == 0 {
		cause = "the container failed"
	}

	switch {
	case t.MaxRetries == task.RetryForever:
		return fmt.Sprintf("attempt %d: %s", attempt+1, cause)

	case t.MaxRetries == 0:
		return cause

	default:
		return fmt.Sprintf("attempt %d of %d: %s", attempt+1, t.MaxRetries+1, cause)
	}
}

// again asks for the container one more time.
func (uc *TaskFailed) again(ctx context.Context, t *task.Task, attempt int) error {
	uc.logger.InfoContext(ctx, "asking for a failed container again",
		"uuid", t.UUID, "name", t.Name, "attempt", attempt, "max_retries", t.MaxRetries)

	// written down before it is asked for, so that a container on its way back
	// is not also taken for one that has drifted.
	t.CurrentState = task.Scheduled
	if _, err := uc.taskRepository.Save(ctx, t); err != nil {
		return err
	}

	return uc.scheduler.On(ctx, t, t.NodeName, attempt)
}

// giveUp stops asking. What the container is, is now also what is expected of
// it, so the runner's own heartbeat leaves it alone until somebody asks for
// something else.
func (uc *TaskFailed) giveUp(ctx context.Context, t *task.Task) error {
	uc.logger.ErrorContext(ctx, "giving up on a container",
		"uuid", t.UUID, "name", t.Name, "max_retries", t.MaxRetries, "reason", t.Reason)

	// only what was still wanted running is given up on: a container that
	// failed on its way to being stopped has arrived where it was going.
	if t.ExpectedState == task.Running {
		t.ExpectedState = task.Failed

		if _, err := uc.taskRepository.Save(ctx, t); err != nil {
			return err
		}
	}

	if !t.Removable() {
		return nil
	}

	// whoever was waiting for this job has been told what became of it by the
	// same failure that got us here, so there is nothing left to keep.
	if _, err := uc.deleteTask.Execute(ctx, &deletetask.Request{UUID: t.UUID, Force: true}); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return err
	}

	return nil
}
