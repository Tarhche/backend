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
	producer       domain.Producer
}

var _ domain.MessageHandler = &Heartbeat{}

func NewHeartbeatHandler(
	taskRepository task.Repository,
	producer domain.Producer,
) *Heartbeat {
	return &Heartbeat{
		taskRepository: taskRepository,
		producer:       producer,
	}
}

func (h *Heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.Heartbeat

	err := json.Unmarshal(data, &heartbeat)
	if err != nil {
		return err
	}

	t, err := h.taskRepository.GetOne(ctx, heartbeat.UUID)
	if err == domain.ErrNotExists {
		return nil
	} else if err != nil {
		return err
	}

	t.ContainerLogs = heartbeat.Logs
	_, err = h.taskRepository.Save(ctx, &t)
	if err != nil {
		return err
	}

	taskState := task.State(heartbeat.State)

	switch taskState {
	case task.Running:
		err = h.publishTaskRan(ctx, &heartbeat)
	case task.Stopped:
		err = h.publishTaskStopped(ctx, &heartbeat)
	case task.Completed:
		err = h.publishTaskCompleted(ctx, &heartbeat)
	case task.Failed:
		err = h.publishTaskFailed(ctx, &heartbeat)
	}

	if err != nil {
		return err
	}

	if task.IsTerminalState(taskState) && t.AutoRemove {
		return h.publishTaskDeleted(ctx, &heartbeat)
	}

	return nil
}

func (uc *Heartbeat) publishTaskRan(ctx context.Context, heartbeat *events.Heartbeat) error {
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

	return uc.producer.Produce(ctx, events.TaskRanName, payload)
}

func (uc *Heartbeat) publishTaskStopped(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskStopped{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskStoppedName, payload)
}

func (uc *Heartbeat) publishTaskCompleted(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskCompleted{
		UUID:     heartbeat.UUID,
		NodeName: heartbeat.NodeName,
		At:       heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskCompletedName, payload)
}

func (uc *Heartbeat) publishTaskFailed(ctx context.Context, heartbeat *events.Heartbeat) error {
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

	return uc.producer.Produce(ctx, events.TaskFailedName, payload)
}

func (uc *Heartbeat) publishTaskDeleted(ctx context.Context, heartbeat *events.Heartbeat) error {
	event := events.TaskDeleted{
		UUID: heartbeat.UUID,
		At:   heartbeat.At,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(ctx, events.TaskDeletedName, payload)
}
