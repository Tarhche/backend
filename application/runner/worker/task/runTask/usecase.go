package runTask

import (
	"context"
	"strconv"

	"errors"

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
		ReadOnly:         request.ReadOnly,

		// kept false so the container's logs and stats survive it exiting; the
		// manager is what decides when a container is removed.
		AutoRemove: false,

		Labels: map[string]string{
			container.TaskUUIDLabelKey: request.UUID,
			container.TaskNameLabelKey: request.Name,
			container.TaskSlugLabelKey: request.Slug,
			container.TaskKindLabelKey: string(request.TaskKind()),
			container.NodeNameLabelKey: uc.nodeName,

			// which attempt this container is, kept where it lasts exactly as
			// long as it means anything: whoever reports on this container
			// reports the failures behind it along with it.
			container.TaskAttemptLabelKey: strconv.Itoa(request.Attempt),
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

	if err := uc.clearEarlierAttempts(ctx, request); err != nil {
		return nil, err
	}

	containerID, err := uc.containerManager.Create(ctx, c)
	if err != nil {
		// the container may already be there: this task was asked for twice,
		// which is what happens when the first attempt was cut short after it
		// had already created one. Taking the one that exists is the outcome
		// that was wanted either way.
		existing, lookupErr := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, request.UUID)
		if lookupErr != nil || len(existing) == 0 {
			return nil, err
		}

		containerID = existing[0].ID
	}

	if err := uc.containerManager.Start(ctx, containerID); err != nil {
		return nil, err
	}

	return &Response{UUID: containerID}, nil
}

// clearEarlierAttempts takes away what is left of an earlier attempt at this
// task, so the attempt about to be made can have the name and the ports back.
//
// Two containers are left where they are. One that is running is already what
// was wanted, whatever attempt it belongs to: a node that was away for a while
// is asked for its containers again, and they are still standing. And one of
// the attempt that was asked for is the same request arriving twice, which is
// started below either way.
func (uc *UseCase) clearEarlierAttempts(ctx context.Context, request *Request) error {
	previous, err := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, request.UUID)
	if err != nil {
		return err
	}

	for _, c := range previous {
		if c.Status == container.StatusRunning || c.Attempt() == request.Attempt {
			continue
		}

		if err := uc.containerManager.Delete(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return err
		}
	}

	return nil
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
