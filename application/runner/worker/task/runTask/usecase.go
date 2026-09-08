package runTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
)

// UseCase runs a container on this node.
type UseCase struct {
	containerManager container.Manager
	networkManager   network.Manager
	validator        domain.Validator
	nodeName         string
}

// NewUseCase creates a new UseCase
func NewUseCase(
	containerManager container.Manager,
	networkManager network.Manager,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		networkManager:   networkManager,
		validator:        validator,
		nodeName:         nodeName,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if err := uc.ensureNetwork(ctx, request); err != nil {
		return nil, err
	}

	c := &container.Container{
		Name:             request.ContainerName(),
		Image:            request.Image,
		Command:          request.Command,
		WorkingDirectory: request.WorkingDir,

		// kept false so the container's logs and stats survive it exiting; the
		// manager is what decides when a container is removed.
		AutoRemove: false,

		Labels: map[string]string{
			container.TaskUUIDLabelKey: request.UUID,
			container.TaskNameLabelKey: request.Name,
			container.TaskSlugLabelKey: request.Slug,
			container.TaskKindLabelKey: string(request.TaskKind()),
			container.NodeNameLabelKey: uc.nodeName,
		},
		Environment:   request.Environment,
		Entrypoint:    request.Entrypoint,
		RestartPolicy: request.RestartPolicy,
		ExposedPorts:  request.ExposedPortSet(),
		PortBindings:  request.PublishedPorts(),
		Networks:      network.Attachments(request.Policy(), request.StackSlug, request.ServiceName),
		ResourceLimits: container.ResourceLimits{
			Cpu:    request.ResourceLimits.Cpu,
			Memory: request.ResourceLimits.Memory,
			Disk:   request.ResourceLimits.Disk,
		},
	}

	containerID, err := uc.containerManager.Create(ctx, c)
	if err != nil {
		return nil, err
	}

	if err := uc.containerManager.Start(ctx, containerID); err != nil {
		return nil, err
	}

	return &Response{UUID: containerID}, nil
}

// ensureNetwork makes the network this container joins exist before it tries to
// join it. A stack's services all run on this node, so the network they share
// is created here too.
func (uc *UseCase) ensureNetwork(ctx context.Context, request *Request) error {
	if request.Policy() == network.PolicyNone {
		return nil
	}

	if len(request.StackSlug) > 0 {
		return uc.networkManager.EnsureStackNetwork(ctx, request.StackSlug)
	}

	return uc.networkManager.EnsureIsolatedNetwork(ctx)
}
