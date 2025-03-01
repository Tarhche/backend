package heartbeatTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type Heartbeat struct {
	taskRepository task.Repository
	publisher      domain.Publisher
}

var _ domain.MessageHandler = &Heartbeat{}

func NewHeartbeatHandler(
	taskRepository task.Repository,
	publisher domain.Publisher,
) *Heartbeat {
	return &Heartbeat{
		taskRepository: taskRepository,
		publisher:      publisher,
	}
}

func (h *Heartbeat) Handle(data []byte) error {
	var heartbeat events.Heartbeat

	err := json.Unmarshal(data, &heartbeat)
	if err != nil {
		return err
	}

	t, err := h.taskRepository.GetOne(heartbeat.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	taskState := task.State(heartbeat.State)

	switch taskState {
	case task.Stopped:
		err = h.publishTaskStopped(&heartbeat)
	case task.Completed:
		err = h.publishTaskCompleted(&heartbeat)
	case task.Failed:
		err = h.publishTaskFailed(&heartbeat)
	}

	if err != nil {
		return err
	}

	if task.IsTerminalState(taskState) && t.AutoRemove {
		err = h.publishTaskDeleted(heartbeat.UUID)
	}

	return err
}

func (uc *Heartbeat) publishTaskStopped(heartbeat *events.Heartbeat) error {
	event := events.TaskStopped{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskDeletedName, payload)
}

func (uc *Heartbeat) publishTaskCompleted(heartbeat *events.Heartbeat) error {
	event := events.TaskCompleted{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskDeletedName, payload)
}

func (uc *Heartbeat) publishTaskFailed(heartbeat *events.Heartbeat) error {
	event := events.TaskFailed{
		UUID:          heartbeat.UUID,
		ContainerUUID: heartbeat.ContainerUUID,
		NodeName:      heartbeat.NodeName,
		FailedAt:      heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskDeletedName, payload)
}

func (uc *Heartbeat) publishTaskDeleted(uuid string) error {
	event := events.TaskDeleted{
		UUID: uuid,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskDeletedName, payload)
}
