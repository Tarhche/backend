package runTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskRan struct {
	taskRepository task.Repository
}

func NewTaskRan(
	taskRepository task.Repository,
) *TaskRan {
	return &TaskRan{
		taskRepository: taskRepository,
	}
}

func (uc *TaskRan) Handle(data []byte) error {
	var taskRan events.TaskRan
	if err := json.Unmarshal(data, &taskRan); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(taskRan.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Running
	if t.State == destinationState {
		return nil
	}

	t.State = destinationState
	t.StartedAt = taskRan.StartedAt
	_, err = uc.taskRepository.Save(&t)

	return err
}
