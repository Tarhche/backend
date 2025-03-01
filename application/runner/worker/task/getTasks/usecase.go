package gettasks

import (
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	containerManager container.Manager
	nodeName         string
}

func NewUseCase(containerManager container.Manager, nodeName string) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		nodeName:         nodeName,
	}
}

func (uc *UseCase) Execute() (*Response, error) {
	allContainers, err := uc.containerManager.GetByLabel(container.NodeNameLabelKey, uc.nodeName)
	if err != nil {
		return nil, err
	}

	tasks := make([]task.Task, len(allContainers))
	for i, c := range allContainers {
		tasks[i] = task.Task{
			UUID:        c.Labels[container.TaskUUIDLabelKey],
			Name:        c.Labels[container.TaskNameLabelKey],
			Image:       c.Image,
			ContainerID: c.ID,
			CreatedAt:   c.CreatedAt,
			State:       uc.containerManager.EvaluateTaskState(c.Status),
		}
	}

	return NewResponse(tasks), nil
}
