package beatHeart

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// UseCase reports what this node is running, so the control plane can follow every
// task's state and learn which of its ports came up.
type UseCase struct {
	taskManager     task.Runtime
	messageProducer domain.Producer
	nodeName        string

	// startedAt is when each task this node holds began running, which is
	// what a task's allowed time is counted from. Docker only tells it on
	// inspection, so it is asked for once per task and remembered.
	startedAt map[string]time.Time

	// exitCodes is what the program in each ended task returned, which is
	// what tells a job that finished from one that fell over. It is asked for
	// the same way, and forgotten as soon as the task runs again.
	exitCodes map[string]int

	logger *slog.Logger
}

func NewUseCase(
	taskManager task.Runtime,
	messageProducer domain.Producer,
	nodeName string,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		taskManager:     taskManager,
		messageProducer: messageProducer,
		nodeName:        nodeName,
		startedAt:       make(map[string]time.Time),
		exitCodes:       make(map[string]int),
		logger:          logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context) error {
	allTasks, err := uc.taskManager.OnNode(ctx, uc.nodeName)
	if err != nil {
		return err
	}

	uc.forgetGone(allTasks)

	for _, c := range allTasks {
		event := events.Heartbeat{
			UUID:        c.TaskUUID,
			Name:        c.TaskName,
			Slug:        c.Slug,
			Kind:        string(c.Kind),
			OwnerUUID:   c.OwnerUUID,
			StackUUID:   c.StackUUID,
			Image:       c.Image,
			ExecutionID: c.ID,
			State:       int(task.EvaluateState(c.Status, c.Kind, uc.exitCode(ctx, &c))),
			NodeName:    uc.nodeName,
			Attempt:     c.Attempt,
			Interactive: c.Interactive,
			Deadline:    uc.deadline(ctx, &c),
			Endpoints:   uc.endpoints(&c),
			Logs:        uc.logs(ctx, &c),
			At:          time.Now(),
		}

		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		if err := uc.messageProducer.Produce(ctx, events.HeartbeatName, payload); err != nil {
			return err
		}
	}

	return nil
}

// deadline is when a task that may only run for so long will have run
// long enough. It is counted from when the task actually started, which
// docker reports on inspection alone, so a task is inspected once and
// what it says is kept for as long as this node holds it.
func (uc *UseCase) deadline(ctx context.Context, c *task.Execution) time.Time {
	ttl := c.TTL

	if ttl <= 0 {
		return time.Time{}
	}

	if started, ok := uc.startedAt[c.ID]; ok {
		if started.IsZero() {
			return time.Time{}
		}

		return started.Add(ttl)
	}

	inspected, err := uc.taskManager.Inspect(ctx, c.ID)
	if err != nil {
		// it will be asked again on the next beat; until then it has no
		// deadline to report rather than a made-up one.
		uc.logger.WarnContext(ctx, "failed to inspect a task for when it started", "error", err)

		return time.Time{}
	}

	if inspected.StartedAt.IsZero() {
		// not running yet: nothing is counting down.
		return time.Time{}
	}

	uc.startedAt[c.ID] = inspected.StartedAt

	return inspected.StartedAt.Add(ttl)
}

// exitCode is what the program in a task returned, for one that has
// ended. Docker only tells it on inspection, so it is asked for once and kept
// until the task runs again or goes away.
func (uc *UseCase) exitCode(ctx context.Context, c *task.Execution) int {
	if !c.Status.Ended() {
		// it may yet end, and what it returns then is not what it returned
		// the last time it ran.
		delete(uc.exitCodes, c.ID)

		return 0
	}

	if code, ok := uc.exitCodes[c.ID]; ok {
		return code
	}

	inspected, err := uc.taskManager.Inspect(ctx, c.ID)
	if err != nil {
		// it will be asked again on the next beat; until then what it returned
		// is unknown, which is not the same as a failure.
		uc.logger.WarnContext(ctx, "failed to inspect a task for what it returned", "error", err)

		return 0
	}

	uc.exitCodes[c.ID] = inspected.ExitCode

	return inspected.ExitCode
}

// forgetGone lets go of what was remembered about tasks this node no
// longer holds.
func (uc *UseCase) forgetGone(held []task.Execution) {
	if len(uc.startedAt) == 0 && len(uc.exitCodes) == 0 {
		return
	}

	ids := make(map[string]struct{}, len(held))
	for i := range held {
		ids[held[i].ID] = struct{}{}
	}

	for id := range uc.startedAt {
		if _, ok := ids[id]; !ok {
			delete(uc.startedAt, id)
		}
	}

	for id := range uc.exitCodes {
		if _, ok := ids[id]; !ok {
			delete(uc.exitCodes, id)
		}
	}
}

// logs collects a task's whole output for the heartbeat to carry.
//
// Only a one-shot job's log travels this way: it is what the caller waiting on
// that job receives when it finishes. A long-running service would make every
// heartbeat carry its entire history, so its output is streamed line by line
// and kept by the control plane instead.
func (uc *UseCase) logs(ctx context.Context, c *task.Execution) []byte {
	if c.Kind == task.KindService {
		return nil
	}

	var buffer bytes.Buffer

	if err := uc.taskManager.Logs(ctx, c.ID, &buffer); err != nil {
		// a task that has not started yet has no logs to read, which is
		// ordinary rather than a failure.
		uc.logger.WarnContext(ctx, "failed to fetch task logs", "error", err)

		return nil
	}

	if buffer.Len() == 0 {
		return nil
	}

	return buffer.Bytes()
}

// endpoints reports which of a task's exposed ports the runtime can reach it
// on. They are read from the runtime every heartbeat because a restarted task
// comes back without them until it is up again.
func (uc *UseCase) endpoints(c *task.Execution) []events.Endpoint {
	endpoints := make([]events.Endpoint, 0, len(c.Endpoints))

	for _, taskPort := range c.Endpoints {
		endpoints = append(endpoints, events.Endpoint{TaskPort: taskPort})
	}

	// a runtime hands them back in no particular order, and the lowest exposed
	// port is the one a bare hostname reaches.
	slices.SortFunc(endpoints, func(a events.Endpoint, b events.Endpoint) int {
		return int(a.TaskPort) - int(b.TaskPort)
	})

	return endpoints
}
