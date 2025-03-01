package runTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

type UseCase struct {
	taskRepository  task.Repository
	asyncCommandBus domain.Publisher
	validator       domain.Validator
}

func NewUseCase(
	taskRepository task.Repository,
	asyncCommandBus domain.Publisher,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		asyncCommandBus: asyncCommandBus,
		validator:       validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	t := task.Task{
		Name:          request.Name,
		State:         task.Created,
		Image:         request.Image,
		PortBindings:  request.ConvertPortBindings(),
		RestartPolicy: request.RestartPolicy,
		RestartCount:  request.RestartCount,
		HealthCheck:   request.HealthCheck,
		AttachStdin:   request.AttachStdin,
		AttachStdout:  request.AttachStdout,
		AttachStderr:  request.AttachStderr,
		Environment:   request.Environment,
		Command:       request.Command,
		Entrypoint:    request.Entrypoint,
		Mounts:        request.ConvertMounts(),
		ResourceLimits: task.ResourceLimits{
			Cpu:    request.ResourceLimits.Cpu,
			Memory: request.ResourceLimits.Memory,
			Disk:   request.ResourceLimits.Disk,
		},
		OwnerUUID: request.OwnerUUID,
	}

	uuid, err := uc.taskRepository.Save(&t)
	if err != nil {
		return nil, err
	}

	if err := uc.publishTaskCreated(uuid); err != nil {
		return nil, err
	}

	return &Response{UUID: uuid}, nil
}

func (uc *UseCase) publishTaskCreated(uuid string) error {
	event := events.TaskCreated{
		UUID: uuid,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Publish(context.Background(), events.TaskCreatedName, payload)
}
