package runTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskFailed struct {
	taskRepository task.Repository
}

func NewTaskFailed(
	taskRepository task.Repository,
) *TaskFailed {
	return &TaskFailed{
		taskRepository: taskRepository,
	}
}

func (uc *TaskFailed) Handle(data []byte) error {
	var taskFailed events.TaskFailed
	if err := json.Unmarshal(data, &taskFailed); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(taskFailed.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Failed
	if t.State == destinationState {
		return nil
	}

	t.State = destinationState
	_, err = uc.taskRepository.Save(&t)

	return err
}
