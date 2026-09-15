package runTask

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase runs a task on this node.
type UseCase struct {
	taskManager    task.Runtime
	networkManager network.Manager
	validator      domain.Validator
	nodeName       string
}

// NewUseCase creates a new UseCase
func NewUseCase(
	taskManager task.Runtime,
	networkManager network.Manager,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		taskManager:    taskManager,
		networkManager: networkManager,
		validator:      validator,
		nodeName:       nodeName,
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

	// the image is made sure of first, so that what a task is allowed to
	// run for is counted from when it runs rather than from when it was asked
	// for: pulling an image it has never seen can take longer than the whole
	// of that.
	if err := uc.taskManager.EnsureImage(ctx, request.Image); err != nil {
		return nil, err
	}

	c := &task.Execution{
		Name:             request.TaskName(),
		Image:            request.Image,
		Command:          request.Command,
		WorkingDirectory: request.WorkingDir,
		ReadOnly:         request.ReadOnly,

		// kept false so the task's logs and stats survive it exiting; the
		// manager is what decides when a task is removed.
		AutoRemove: false,

		// what it is running, which the runtime keeps with it: it is how this
		// node says what it is holding, and whose, without asking anything
		// that keeps records.
		TaskUUID: request.UUID,
		TaskName: request.Name,
		Slug:     request.Slug,
		Kind:     request.TaskKind(),
		NodeName: uc.nodeName,

		// whose it is, so that the node holding it can answer for itself who
		// may be let in.
		OwnerUUID: request.OwnerUUID,

		// which attempt this is, kept where it lasts exactly as long as it
		// means anything: whoever reports on this task reports the failures
		// behind it along with it.
		Attempt: request.Attempt,

		// whether anybody is watching it run, which is what says a report
		// about it is worth passing on as it happens.
		Interactive: request.Interactive,

		// how long it may run for once it is up. What that is counted from is
		// not this node's to decide: the run itself says when it started.
		TTL: request.TTL,

		Environment:   request.Environment,
		Entrypoint:    request.Entrypoint,
		RestartPolicy: request.RestartPolicy,
		ExposedPorts:  request.ExposedPortSet(),
		PortBindings:  request.PublishedPorts(),
		Networks:      network.Attachments(request.Policy(), request.StackSlug, request.ServiceName),
		ResourceLimits: task.ResourceLimits{
			Cpu:    request.ResourceLimits.Cpu,
			Memory: request.ResourceLimits.Memory,
			Disk:   request.ResourceLimits.Disk,
		},
	}

	if len(request.StackUUID) > 0 {
		c.StackUUID = request.StackUUID
	}

	if err := uc.clearEarlierAttempts(ctx, request); err != nil {
		return nil, err
	}

	taskID, err := uc.taskManager.Create(ctx, c)
	if err != nil {
		// the task may already be there: this task was asked for twice,
		// which is what happens when the first attempt was cut short after it
		// had already created one. Taking the one that exists is the outcome
		// that was wanted either way.
		existing, lookupErr := uc.taskManager.Of(ctx, request.UUID)
		if lookupErr != nil || len(existing) == 0 {
			return nil, err
		}

		taskID = existing[0].ID
	}

	if err := uc.taskManager.Start(ctx, taskID); err != nil {
		return nil, err
	}

	return &Response{UUID: taskID}, nil
}

// clearEarlierAttempts takes away what is left of an earlier attempt at this
// task, so the attempt about to be made can have the name and the ports back.
//
// Two tasks are left where they are. One that is running is already what
// was wanted, whatever attempt it belongs to: a node that was away for a while
// is asked for its tasks again, and they are still standing. And one of
// the attempt that was asked for is the same request arriving twice, which is
// started below either way.
func (uc *UseCase) clearEarlierAttempts(ctx context.Context, request *Request) error {
	previous, err := uc.taskManager.Of(ctx, request.UUID)
	if err != nil {
		return err
	}

	for _, c := range previous {
		if c.Status == task.StatusRunning || c.Attempt == request.Attempt {
			continue
		}

		if err := uc.taskManager.Delete(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return err
		}
	}

	return nil
}

// ensureNetwork makes the network this task joins exist before it tries to
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
