package runTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskCompleted struct {
	taskRepository task.Repository
}

func NewTaskCompleted(
	taskRepository task.Repository,
) *TaskCompleted {
	return &TaskCompleted{
		taskRepository: taskRepository,
	}
}

func (uc *TaskCompleted) Handle(ctx context.Context, data []byte) error {
	var taskCompleted events.TaskCompleted
	if err := json.Unmarshal(data, &taskCompleted); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskCompleted.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Completed
	if t.State == destinationState {
		return nil
	}

	t.State = destinationState
	t.FinishedAt = taskCompleted.At
	_, err = uc.taskRepository.Save(ctx, &t)

	return err
}
