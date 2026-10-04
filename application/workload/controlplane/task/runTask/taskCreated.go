package runTask

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
	nominatedNodesLimit = 10
)

type TaskCreated struct {
	taskRepository task.Repository
	nodeRepository node.Repository
	placement      task.Scheduler
	scheduler      *schedule.Scheduler
	logger         *slog.Logger
}

func NewTaskCreated(
	taskRepository task.Repository,
	nodeRepository node.Repository,
	placement task.Scheduler,
	scheduler *schedule.Scheduler,
	logger *slog.Logger,
) *TaskCreated {
	return &TaskCreated{
		taskRepository: taskRepository,
		nodeRepository: nodeRepository,
		placement:      placement,
		scheduler:      scheduler,
		logger:         logger,
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

// pickNode chooses where a task runs: where it was nominated, or wherever
// there is room.
func (uc *TaskCreated) pickNode(ctx context.Context, t *task.Task) (node.Node, error) {
	if len(t.NodeName) > 0 {
		return node.Node{Name: t.NodeName}, nil
	}

	return uc.anyNode(ctx, t)
}

// anyNode is wherever there is room for a task that is not held to a
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
