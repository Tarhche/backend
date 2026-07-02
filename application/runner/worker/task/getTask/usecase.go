package getTask

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	containerManager container.Manager
	logger           *slog.Logger
}

func NewUseCase(containerManager container.Manager, logger *slog.Logger) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		logger:           logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context, uuid string) (*Response, error) {
	containers, err := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, uuid)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, errors.New("task not found")
	}

	c := containers[len(containers)-1]

	var logsBuffer bytes.Buffer
	if err := uc.containerManager.Logs(ctx, c.ID, &logsBuffer); err != nil {
		uc.logger.Warn("failed to fetch container logs", "error", err) // there are some cases that the container is not started yet and we can't get the logs
	}

	var logs []byte
	if logsBuffer.Len() > 0 {
		logs = make([]byte, logsBuffer.Len())
		if _, err := logsBuffer.Read(logs); err != nil {
			return nil, err
		}
	}

	t := task.Task{
		UUID:          c.Labels[container.TaskUUIDLabelKey],
		Name:          c.Labels[container.TaskNameLabelKey],
		Image:         c.Image,
		ContainerID:   c.ID,
		ContainerLogs: logs,
		CreatedAt:     c.CreatedAt,
		State:         uc.containerManager.EvaluateTaskState(c.Status),
	}

	return NewResponse(&t), nil
}
