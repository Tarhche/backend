package runTask

import (
	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// RunTask runs a task
type UseCase struct {
	containerManager container.Manager
	validator        domain.Validator
	nodeName         string
}

// NewUseCase creates a new UseCase
func NewUseCase(
	containerManager container.Manager,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		validator:        validator,
		nodeName:         nodeName,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	uuid, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	c := &container.Container{
		Name:       request.Name + "-" + uuid.String(),
		Image:      request.Image,
		Command:    request.Command,
		AutoRemove: false, // should be false be able to collect container stats and logs. the manager will handle removal of a container.
		Labels: map[string]string{
			container.TaskUUIDLabelKey: request.UUID,
			container.TaskNameLabelKey: request.Name,
			container.NodeNameLabelKey: uc.nodeName,
		},
		Environment:   request.Environment,
		Entrypoint:    request.Entrypoint,
		RestartPolicy: request.RestartPolicy,
		ResourceLimits: container.ResourceLimits{
			Cpu:    request.ResourceLimits.Cpu,
			Memory: request.ResourceLimits.Memory,
			Disk:   request.ResourceLimits.Disk,
		},
	}

	containerID, err := uc.containerManager.Create(c)
	if err != nil {
		return nil, err
	}

	if err := uc.containerManager.Start(containerID); err != nil {
		return nil, err
	}

	return &Response{UUID: containerID}, nil
}
