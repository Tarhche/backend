package runTask

import (
	"context"
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

func (uc *TaskRan) Handle(ctx context.Context, data []byte) error {
	var taskRan events.TaskRan
	if err := json.Unmarshal(data, &taskRan); err != nil {
		return err
	}

	t, err := uc.taskRepository.GetOne(ctx, taskRan.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	// the endpoints are worth recording even when the state has not moved: a
	// restarted container is published on new host ports.
	t.NodeName = taskRan.NodeName
	t.ContainerID = taskRan.ContainerUUID
	t.Endpoints = toEndpoints(taskRan.Endpoints)

	destinationState := task.Running
	if t.State != destinationState {
		t.State = destinationState
		t.StartedAt = taskRan.StartedAt
	}

	_, err = uc.taskRepository.Save(ctx, &t)

	return err
}

// toEndpoints reads the addresses a worker published a container on.
func toEndpoints(endpoints []events.Endpoint) []task.Endpoint {
	result := make([]task.Endpoint, len(endpoints))
	for i, e := range endpoints {
		result[i] = task.Endpoint{
			ContainerPort: e.ContainerPort,
			Host:          e.Host,
			HostPort:      e.HostPort,
		}
	}

	return result
}
