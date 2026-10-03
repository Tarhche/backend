package runTask

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/slug"
)

type UseCase struct {
	taskRepository  task.Repository
	asyncCommandBus domain.Producer
	validator       domain.Validator

	// classes are the runtime classes a task may be run with, and the one a
	// task naming none is given.
	classes allowed.Classes
}

func NewUseCase(
	taskRepository task.Repository,
	asyncCommandBus domain.Producer,
	validator domain.Validator,
	classes allowed.Classes,
) *UseCase {
	return &UseCase{
		taskRepository:  taskRepository,
		asyncCommandBus: asyncCommandBus,
		validator:       validator,
		classes:         classes,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(admission{Request: request, classes: uc.classes}); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	// the slug is the name this task is addressed by from outside: it is
	// unique, it survives restarts, and it is the left-most label of the
	// hostname its ports are served on.
	taskSlug, err := slug.Generate(request.Name)
	if err != nil {
		return nil, err
	}

	t := task.Task{
		Name: request.Name,
		Slug: taskSlug,
		Kind: request.TaskKind(),

		// the class is resolved here, once, and stored: what the default is
		// later is no business of a task that already exists.
		Runtime: uc.classes.Resolve(request.Runtime),

		StackUUID:     request.StackUUID,
		ServiceName:   request.ServiceName,
		CurrentState:  task.Created,
		ExpectedState: task.Running,
		Image:         request.Image,
		AutoRemove:    request.AutoRemove,
		PortBindings:  request.ConvertPortBindings(),
		ExposedPorts:  request.ExposedPorts,
		NetworkPolicy: request.Policy(),
		RestartPolicy: request.RestartPolicy,
		RestartCount:  request.RestartCount,
		HealthCheck:   request.HealthCheck,
		AttachStdin:   request.AttachStdin,
		AttachStdout:  request.AttachStdout,
		AttachStderr:  request.AttachStderr,
		Environment:   request.Environment,
		Command:       request.Command,
		Entrypoint:    request.Entrypoint,
		WorkingDir:    request.WorkingDir,
		ReadOnly:      request.ReadOnly,
		Interactive:   request.Interactive,
		MaxRetries:    request.Retries(),
		TTL:           request.TTL,
		Mounts:        request.ConvertMounts(),
		ResourceLimits: task.ResourceLimits{
			Cpu:    request.ResourceLimits.Cpu,
			Memory: request.ResourceLimits.Memory,
			Disk:   request.ResourceLimits.Disk,
		},
		NodeName:  request.NominatedNode,
		OwnerUUID: request.OwnerUUID,
	}

	uuid, err := uc.taskRepository.Save(ctx, &t)
	if err != nil {
		return nil, err
	}

	if err := uc.publishTaskCreated(ctx, uuid); err != nil {
		return nil, err
	}

	return &Response{UUID: uuid, Slug: taskSlug}, nil
}

func (uc *UseCase) publishTaskCreated(ctx context.Context, uuid string) error {
	event := &events.TaskCreated{
		UUID: uuid,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.asyncCommandBus.Produce(ctx, events.TaskCreatedName, payload)
}
