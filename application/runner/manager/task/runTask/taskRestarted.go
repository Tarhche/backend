package runTask

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// TaskRestarted marks a container running again after a restart, and records
// the host ports it came back on, which a restart may change.
type TaskRestarted struct {
	taskRepository task.Repository
}

var _ domain.MessageHandler = &TaskRestarted{}

func NewTaskRestarted(taskRepository task.Repository) *TaskRestarted {
	return &TaskRestarted{taskRepository: taskRepository}
}

func (uc *TaskRestarted) Handle(ctx context.Context, data []byte) error {
	var taskRestarted events.TaskRestarted
	if err := json.Unmarshal(data, &taskRestarted); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskRestarted.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	t.State = task.Running
	t.NodeName = taskRestarted.NodeName
	t.ContainerID = taskRestarted.ContainerUUID
	t.StartedAt = taskRestarted.At
	t.FinishedAt = time.Time{}

	_, err = uc.taskRepository.Save(ctx, &t)

	return err
}
