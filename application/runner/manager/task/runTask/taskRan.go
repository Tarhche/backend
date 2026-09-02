package runTask

import (
	"context"
	"encoding/json"
	"slices"

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

	endpoints := toEndpoints(taskRan.Endpoints)

	// a running container heartbeats several times a second, and each beat
	// reaches here. Only what has actually changed is worth a write: the
	// endpoints, because a restarted container comes back on new host ports,
	// and the state, the first time it comes up.
	changed := t.NodeName != taskRan.NodeName ||
		t.ContainerID != taskRan.ContainerUUID ||
		!slices.Equal(t.Endpoints, endpoints)

	t.NodeName = taskRan.NodeName
	t.ContainerID = taskRan.ContainerUUID
	t.Endpoints = endpoints

	if t.State != task.Running {
		t.State = task.Running
		t.StartedAt = taskRan.StartedAt
		changed = true
	}

	if !changed {
		return nil
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
