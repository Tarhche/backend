package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	nominatedNodesLimit = 10
)

type TaskCreated struct {
	taskRepository  task.Repository
	nodeRepository  node.Repository
	stackRepository stack.Repository
	placement       task.Scheduler
	scheduler       *schedule.Scheduler
	logger          *slog.Logger
}

func NewTaskCreated(
	taskRepository task.Repository,
	nodeRepository node.Repository,
	stackRepository stack.Repository,
	placement task.Scheduler,
	scheduler *schedule.Scheduler,
	logger *slog.Logger,
) *TaskCreated {
	return &TaskCreated{
		taskRepository:  taskRepository,
		nodeRepository:  nodeRepository,
		stackRepository: stackRepository,
		placement:       placement,
		scheduler:       scheduler,
		logger:          logger,
	}
}

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
	if err != nil {
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

// pickNode chooses where a container runs.
//
// A service of a stack has that choice made for it: everything in a stack
// shares one private network, and a bridge is local to the node that created
// it, so a stack runs on one node or it does not run. Anything else goes where
// it was nominated, or wherever there is room.
func (uc *TaskCreated) pickNode(ctx context.Context, t *task.Task) (node.Node, error) {
	if len(t.StackUUID) > 0 {
		return uc.stackNode(ctx, t)
	}

	if len(t.NodeName) > 0 {
		return node.Node{Name: t.NodeName}, nil
	}

	return uc.anyNode(ctx, t)
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
		return uc.anyNode(ctx, t)
	} else if err != nil {
		return node.Node{}, err
	}

	if len(s.NodeName) > 0 {
		return node.Node{Name: s.NodeName}, nil
	}

	selected, err := uc.anyNode(ctx, t)
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

// anyNode is wherever there is room for a container that is not held to a
// place by anything else.
func (uc *TaskCreated) anyNode(ctx context.Context, t *task.Task) (node.Node, error) {
	nodes, err := uc.getHealthyNodes(ctx)
	if err != nil {
		return node.Node{}, err
	}

	if len(nodes) == 0 {
		return node.Node{}, node.ErrNoNodesAvailable
	}

	return uc.placement.Pick(t, nodes), nil
}

func (uc *TaskCreated) getHealthyNodes(ctx context.Context) ([]node.Node, error) {
	nodes, err := uc.nodeRepository.GetAll(ctx, 0, nominatedNodesLimit)
	if err != nil {
		return nil, err
	}

	j := 0
	for i := range nodes {
		if nodes[i].LastHeartbeatAt.After(time.Now().Add(-3 * time.Second)) {
			nodes[j] = nodes[i]
			j++
		}
	}
	nodes = nodes[:j:j]

	return nodes, nil
}
