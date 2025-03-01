package stopTask

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskStopped struct {
	taskRepository task.Repository
}

func NewTaskStopped(
	taskRepository task.Repository,
) *TaskStopped {
	return &TaskStopped{
		taskRepository: taskRepository,
	}
}

func (uc *TaskStopped) Handle(data []byte) error {
	var taskStopped events.TaskStopped
	if err := json.Unmarshal(data, &taskStopped); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(taskStopped.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	destinationState := task.Stopped
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
