package runTask

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type TaskRunRequested struct {
	usecase *UseCase
	logger  *slog.Logger
}

func NewTaskRunRequested(
	usecase *UseCase,
	logger *slog.Logger,
) *TaskRunRequested {
	return &TaskRunRequested{
		usecase: usecase,
		logger:  logger,
	}
}

func (uc *TaskRunRequested) Handle(ctx context.Context, data []byte) error {
	uc.logger.Info("task run requested event received", "data", string(data))

	var event events.TaskRunRequested
	if err := json.Unmarshal(data, &event); err != nil {
		uc.logger.Error("error unmarshalling request", "error", err)

		return nil
	}

	portBindings := make(map[uint][]PortBinding, len(event.PortBindings))
	for hostPort, containerPorts := range event.PortBindings {
		portBindings[hostPort] = make([]PortBinding, len(containerPorts))
		for i, containerPort := range containerPorts {
			portBindings[hostPort][i] = PortBinding{
				HostIP:   containerPort.HostIP,
				HostPort: uint(containerPort.HostPort),
			}
		}
	}

	mounts := make([]Mount, len(event.Mounts))
	for i, mount := range event.Mounts {
		mounts[i] = Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			Type:     mount.Type,
			ReadOnly: mount.ReadOnly,
		}
	}

	resourceLimits := ResourceLimits{
		Cpu:    event.ResourceLimits.Cpu,
		Memory: event.ResourceLimits.Memory,
		Disk:   event.ResourceLimits.Disk,
	}

	request := &Request{
		Name:           event.Name,
		Image:          event.Image,
		AutoRemove:     event.AutoRemove,
		PortBindings:   portBindings,
		RestartPolicy:  event.RestartPolicy,
		RestartCount:   event.RestartCount,
		HealthCheck:    event.HealthCheck,
		AttachStdin:    event.AttachStdin,
		AttachStdout:   event.AttachStdout,
		AttachStderr:   event.AttachStderr,
		Environment:    event.Environment,
		Command:        event.Command,
		Entrypoint:     event.Entrypoint,
		Mounts:         mounts,
		ResourceLimits: resourceLimits,
		OwnerUUID:      event.OwnerUUID,
	}

	// TODO: using usecase in handler ? (is this a good idea?)
	response, err := uc.usecase.Execute(ctx, request)
	if len(response.ValidationErrors) > 0 {
		uc.logger.Warn("validation errors", "validationErrors", response.ValidationErrors)
	}

	if err != nil {
		uc.logger.Error("error running task", "error", err)
	}

	return err
}
