package runTask

import (
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

func (uc *TaskCompleted) Handle(data []byte) error {
	var taskCompleted events.TaskCompleted
	if err := json.Unmarshal(data, &taskCompleted); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(taskCompleted.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Completed
	if t.State == destinationState {
		return nil
	}

	if !task.ValidStateTransition(t.State, destinationState) {
		return task.ErrInvalidStateTransition
	}

	t.State = destinationState
	_, err = uc.taskRepository.Save(&t)

	return err
}
