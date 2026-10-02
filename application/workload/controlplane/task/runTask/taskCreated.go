package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
)

type TaskCreated struct {
	taskRepository  task.Repository
	stackRepository stack.Repository
	placement       *placement.Placement
	scheduler       *schedule.Scheduler

	// asyncCommandBus is where a task that no node will ever run is said to
	// have failed, the way a node says so of one it could not run.
	asyncCommandBus domain.Producer

	logger *slog.Logger
}

func NewTaskCreated(
	taskRepository task.Repository,
	stackRepository stack.Repository,
	placement *placement.Placement,
	scheduler *schedule.Scheduler,
	asyncCommandBus domain.Producer,
	logger *slog.Logger,
) *TaskCreated {
	return &TaskCreated{
		taskRepository:  taskRepository,
		stackRepository: stackRepository,
		placement:       placement,
		scheduler:       scheduler,
		asyncCommandBus: asyncCommandBus,
		logger:          logger,
	}
}

// Handle places a task that has been created.
//
// Nothing that stands in the way of placing it is returned as an error: an
// error here is redelivered at once, and for as long as it keeps failing, which
// helps nothing that waiting would not. A class no node offers at all fails the
// task; a class no node can run it with right now leaves it as it is, and the
// workload's own heartbeat asks for it to be placed again.
func (uc *TaskCreated) Handle(ctx context.Context, data []byte) error {
	var taskCreated events.TaskCreated
	if err := json.Unmarshal(data, &taskCreated); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskCreated.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Scheduled
	if t.CurrentState == destinationState {
		return nil
	}

	selectedNode, err := uc.pickNode(ctx, &t)
	switch {
	case errors.Is(err, placement.ErrNoNodeOffersRuntime):
		return uc.fail(ctx, &t, runtime.ReasonNoNodeOffersRuntime)

	case errors.Is(err, placement.ErrNoNodeReady):
		uc.logger.InfoContext(ctx, "no node can run a task right now; it waits to be placed again",
			"uuid", t.UUID, "name", t.Name, "runtime", t.Runtime.OrSysbox().String())

		return nil

	case err != nil:
		return err
	}

	t.CurrentState = destinationState
	t.NodeName = selectedNode.Name
	if _, err = uc.taskRepository.Save(ctx, &t); err != nil {
		return err
	}

	// the first attempt at it: nothing has failed yet.
	return uc.scheduler.On(ctx, &t, selectedNode.Name, 0)
}

// pickNode chooses where a task runs.
//
// A service of a stack has that choice made for it: everything in a stack
// shares one private network, and a bridge is local to the node that created
// it, so a stack runs on one node or it does not run. Anything else goes where
// it was nominated, or wherever placement finds a node that can run it.
func (uc *TaskCreated) pickNode(ctx context.Context, t *task.Task) (node.Node, error) {
	if len(t.StackUUID) > 0 {
		return uc.stackNode(ctx, t)
	}

	if len(t.NodeName) > 0 {
		return node.Node{Name: t.NodeName}, nil
	}

	return uc.placement.Place(ctx, t)
}

// stackNode is the one node a stack's services all run on.
//
// It is read from the stack rather than from the service, so that services
// asked for at different moments, by different paths, all end up in the same
// place — and written back to the stack when it does not have one yet, so that
// the first service to be placed decides for the rest.
func (uc *TaskCreated) stackNode(ctx context.Context, t *task.Task) (node.Node, error) {
	s, err := uc.stackRepository.GetOne(ctx, t.StackUUID)
	if errors.Is(err, domain.ErrNotExists) {
		// there is no stack to keep it with any more.
		return uc.placement.Place(ctx, t)
	} else if err != nil {
		return node.Node{}, err
	}

	if len(s.NodeName) > 0 {
		return node.Node{Name: s.NodeName}, nil
	}

	selected, err := uc.placement.Place(ctx, t)
	if err != nil {
		return node.Node{}, err
	}

	s.NodeName = selected.Name
	if _, err := uc.stackRepository.Save(ctx, &s); err != nil {
		return node.Node{}, err
	}

	uc.logger.InfoContext(ctx, "a stack was placed", "stack", s.UUID, "node", selected.Name)

	return selected, nil
}

// fail says a task has failed for a reason no node will ever report, since no
// node will ever hold it.
//
// It is said the way a node says it, as a TaskFailed, rather than written down
// here, so that it goes where every other failure goes: into the task's record
// and log, through the giving up that takes away a job nobody will run, and out
// to whoever is waiting on the task — a code-runner page, a dashboard. No
// further attempt is coming, whatever the task was worth, so the message says
// this one was the last.
func (uc *TaskCreated) fail(ctx context.Context, t *task.Task, reason string) error {
	uc.logger.WarnContext(ctx, "no node offers the class a task is run with",
		"uuid", t.UUID, "name", t.Name, "runtime", t.Runtime.OrSysbox().String())

	payload, err := json.Marshal(events.TaskFailed{
		UUID:       t.UUID,
		Name:       t.Name,
		OwnerUUID:  t.OwnerUUID,
		At:         time.Now(),
		Attempt:    0,
		MaxRetries: 0,
		Reason:     reason,
	})
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskFailedName, payload)
}
