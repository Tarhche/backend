package watch

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	stackEvents "github.com/khanzadimahdi/testproject/domain/runner/stack/events"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// Changes turns what the runner says about its tasks into what the clients
// watching them are told.
//
// Each of its handlers is one of the runner's messages. They are subscribed to
// rather than consumed: every replica hears all of them and answers for the
// clients it holds, so which replica a watch was opened on does not matter.
type Changes struct {
	watchers      *Watchers
	runner        runnerManager.Client
	owners        *presenter.Directory
	replyer       domain.Replyer
	ingressDomain string
	seen          *seen
	logger        *slog.Logger
}

func NewChanges(
	watchers *Watchers,
	runner runnerManager.Client,
	ownerDirectory *presenter.Directory,
	replyer domain.Replyer,
	ingressDomain string,
	logger *slog.Logger,
) *Changes {
	return &Changes{
		watchers:      watchers,
		runner:        runner,
		owners:        ownerDirectory,
		replyer:       replyer,
		ingressDomain: ingressDomain,
		seen:          newSeen(),
		logger:        logger,
	}
}

// Heartbeat is a node saying what it is holding, which is where a watch gets
// almost all of its news.
func (c *Changes) Heartbeat(ctx context.Context, data []byte) error {
	if !c.watchers.Watching() {
		return nil
	}

	var beat taskEvents.Heartbeat
	if err := json.Unmarshal(data, &beat); err != nil {
		return nil
	}

	state := task.State(beat.State)

	task := Task{
		UUID:      beat.UUID,
		Name:      beat.Name,
		Slug:      beat.Slug,
		Kind:      beat.Kind,
		State:     state.String(),
		Image:     beat.Image,
		StackUUID: beat.StackUUID,
		NodeName:  beat.NodeName,
		Attempt:   beat.Attempt,
		Endpoints: c.endpoints(&beat, state),
		Deadline:  deadline(&beat, state),
		At:        beat.At,
	}

	if !c.seen.changed(beat.UUID, &task, beat.OwnerUUID, beat.StackUUID) {
		return nil
	}

	c.taskChanged(ctx, &task, beat.OwnerUUID)
	c.stackChanged(ctx, beat.StackUUID)

	return nil
}

// Scheduled is a task that now exists. It is what puts one on a dashboard
// before any node has it, and so before anything beats for it.
func (c *Changes) Scheduled(ctx context.Context, data []byte) error {
	if !c.watchers.Watching() {
		return nil
	}

	var scheduled taskEvents.TaskScheduled
	if err := json.Unmarshal(data, &scheduled); err != nil {
		return nil
	}

	task := Task{
		UUID:      scheduled.UUID,
		Name:      scheduled.Name,
		Slug:      scheduled.Slug,
		Kind:      scheduled.Kind,
		State:     task.Scheduled.String(),
		Image:     scheduled.Image,
		StackUUID: scheduled.StackUUID,
		NodeName:  scheduled.NominatedNode,
		Attempt:   scheduled.Attempt,
		At:        time.Now(),
	}

	c.seen.changed(scheduled.UUID, &task, scheduled.OwnerUUID, scheduled.StackUUID)

	c.taskChanged(ctx, &task, scheduled.OwnerUUID)
	c.stackChanged(ctx, scheduled.StackUUID)

	return nil
}

// Failed is a task that could not be run at all, which no node holds and
// nothing therefore beats for.
func (c *Changes) Failed(ctx context.Context, data []byte) error {
	if !c.watchers.Watching() {
		return nil
	}

	var failed taskEvents.TaskFailed
	if err := json.Unmarshal(data, &failed); err != nil {
		return nil
	}

	was := c.seen.of(failed.UUID)

	ownerUUID := failed.OwnerUUID
	if len(ownerUUID) == 0 {
		ownerUUID = was.ownerUUID
	}

	task := Task{
		UUID:     failed.UUID,
		Name:     failed.Name,
		State:    task.Failed.String(),
		NodeName: failed.NodeName,
		Attempt:  failed.Attempt,
		Reason:   failed.Reason,
		At:       failed.At,
	}

	c.seen.changed(failed.UUID, &task, ownerUUID, was.stackUUID)

	c.taskChanged(ctx, &task, ownerUUID)
	c.stackChanged(ctx, was.stackUUID)

	return nil
}

// Deleted is a task that is gone. It says only which one it was, so it is
// told to everybody watching: a watch that never saw it makes nothing of being
// told about it.
func (c *Changes) Deleted(ctx context.Context, data []byte) error {
	if !c.watchers.Watching() {
		return nil
	}

	var deleted taskEvents.TaskDeleted
	if err := json.Unmarshal(data, &deleted); err != nil {
		return nil
	}

	was := c.seen.forget(deleted.UUID)

	c.tell(ctx, c.watchers.EveryTaskWatch(), &TaskChange{Kind: kindDeleted, UUID: deleted.UUID})
	c.stackChanged(ctx, was.stackUUID)

	return nil
}

// StackDeleted is a stack that is gone, with the services that were in it.
func (c *Changes) StackDeleted(ctx context.Context, data []byte) error {
	if !c.watchers.Watching() {
		return nil
	}

	var deleted stackEvents.StackDeleted
	if err := json.Unmarshal(data, &deleted); err != nil {
		return nil
	}

	c.tell(ctx, c.watchers.EveryStackWatch(), &StackChange{Kind: kindDeleted, UUID: deleted.UUID})

	return nil
}

// taskChanged tells the watches a task belongs to what it is now.
func (c *Changes) taskChanged(ctx context.Context, task *Task, ownerUUID string) {
	c.tell(ctx, c.watchers.Tasks(ownerUUID), &TaskChange{
		Kind: kindChanged,
		UUID: task.UUID,
		Task: task,
	})
}

// stackChanged reads the stack one of its services changed and tells the
// watches following it. A stack has no report of its own, so this is the only
// place one comes from — and it is read only while somebody is watching stacks.
func (c *Changes) stackChanged(ctx context.Context, stackUUID string) {
	if len(stackUUID) == 0 || len(c.watchers.EveryStackWatch()) == 0 {
		return
	}

	s, err := c.runner.Stack(ctx, stackUUID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotExists) {
			c.logger.ErrorContext(ctx, "error on reading a stack a service changed", "error", err, "uuid", stackUUID)
		}

		return
	}

	people, err := c.owners.Of(ctx, s.OwnerUUID)
	if err != nil {
		c.logger.ErrorContext(ctx, "error on reading who a stack belongs to", "error", err, "uuid", stackUUID)

		return
	}

	stack := presenter.NewStack(s, c.ingressDomain, people)

	c.tell(ctx, c.watchers.Stacks(s.OwnerUUID), &StackChange{Kind: kindChanged, UUID: s.UUID, Stack: &stack})
}

// tell carries one change to the watches it is news for.
func (c *Changes) tell(ctx context.Context, requests []string, change any) {
	if len(requests) == 0 {
		return
	}

	payload, err := json.Marshal(change)
	if err != nil {
		c.logger.ErrorContext(ctx, "error on marshalling a change", "error", err)

		return
	}

	for _, requestID := range requests {
		if err := c.replyer.Reply(ctx, &domain.Reply{
			RequestID: requestID,
			Kind:      domain.ReplyChunk,
			Payload:   payload,
		}); err != nil {
			c.logger.ErrorContext(ctx, "error on telling a watch what changed", "error", err, "requestID", requestID)
		}
	}
}

// endpoints are the addresses a task's ports are served on, which it has
// only while it is up.
func (c *Changes) endpoints(beat *taskEvents.Heartbeat, state task.State) []presenter.Endpoint {
	if state != task.Running || len(beat.Slug) == 0 {
		return nil
	}

	endpoints := make([]task.Endpoint, len(beat.Endpoints))
	for i, e := range beat.Endpoints {
		endpoints[i] = task.Endpoint{TaskPort: e.TaskPort}
	}

	return presenter.NewEndpoints(task.Task{Slug: beat.Slug, Endpoints: endpoints}, c.ingressDomain)
}

// deadline is when a task that may only run for so long will be stopped.
// One that is not running has none left to report.
func deadline(beat *taskEvents.Heartbeat, state task.State) *time.Time {
	if state != task.Running || beat.Deadline.IsZero() {
		return nil
	}

	at := beat.Deadline

	return &at
}
