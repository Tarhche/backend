package gettasks

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	taskManager task.Runtime
	nodeName    string
}

func NewUseCase(taskManager task.Runtime, nodeName string) *UseCase {
	return &UseCase{
		taskManager: taskManager,
		nodeName:    nodeName,
	}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	allTasks, err := uc.taskManager.OnNode(ctx, uc.nodeName)
	if err != nil {
		return nil, err
	}

	tasks := make([]task.Task, len(allTasks))
	for i, c := range allTasks {
		tasks[i] = task.Task{
			UUID:         c.TaskUUID,
			Name:         c.TaskName,
			Image:        c.Image,
			ExecutionID:  c.ID,
			CreatedAt:    c.CreatedAt,
			CurrentState: task.EvaluateState(c.Status, c.Kind, c.ExitCode),
		}
	}

	return NewResponse(tasks), nil
}
