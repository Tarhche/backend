package getTask

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	containerManager container.Manager
}

func NewUseCase(containerManager container.Manager) *UseCase {
	return &UseCase{
		containerManager: containerManager,
	}
}

func (uc *UseCase) Execute(uuid string) (*Response, error) {
	containers, err := uc.containerManager.GetByLabel(container.TaskUUIDLabelKey, uuid)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, errors.New("task not found")
	}

	c := containers[len(containers)-1]

	t := task.Task{
		UUID:        c.Labels[container.TaskUUIDLabelKey],
		Name:        c.Labels[container.TaskNameLabelKey],
		Image:       c.Image,
		ContainerID: c.ID,
		CreatedAt:   c.CreatedAt,
		State:       uc.containerManager.EvaluateTaskState(c.Status),
	}

	return NewResponse(&t), nil
}
