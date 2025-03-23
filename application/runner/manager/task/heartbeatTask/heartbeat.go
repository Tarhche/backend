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

	t.ContainerLogs = heartbeat.Logs
	_, err = h.taskRepository.Save(&t)
	if err != nil {
		return err
	}

	taskState := task.State(heartbeat.State)

	switch taskState {
	case task.Running:
		err = h.publishTaskRan(&heartbeat)
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
		return h.publishTaskDeleted(&heartbeat)
	}

	return nil
}

func (uc *Heartbeat) publishTaskRan(heartbeat *events.Heartbeat) error {
	event := events.TaskRan{
		UUID:          heartbeat.UUID,
		NodeName:      heartbeat.NodeName,
		ContainerUUID: heartbeat.ContainerUUID,
		StartedAt:     heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskRanName, payload)
}

func (uc *Heartbeat) publishTaskStopped(heartbeat *events.Heartbeat) error {
	event := events.TaskStopped{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskStoppedName, payload)
}

func (uc *Heartbeat) publishTaskCompleted(heartbeat *events.Heartbeat) error {
	event := events.TaskCompleted{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskCompletedName, payload)
}

func (uc *Heartbeat) publishTaskFailed(heartbeat *events.Heartbeat) error {
	event := events.TaskFailed{
		UUID:          heartbeat.UUID,
		ContainerUUID: heartbeat.ContainerUUID,
		NodeName:      heartbeat.NodeName,
		At:            heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskFailedName, payload)
}

func (uc *Heartbeat) publishTaskDeleted(heartbeat *events.Heartbeat) error {
	event := events.TaskDeleted{
		UUID: heartbeat.UUID,
		At:   heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.publisher.Publish(context.Background(), events.TaskDeletedName, payload)
}
