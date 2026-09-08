package attachTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// UseCase opens a command inside a task's container and hands back the stream
// it runs on. Closing that stream is what ends the command.
type UseCase struct {
	containerManager container.Manager
	validator        domain.Validator
}

func NewUseCase(containerManager container.Manager, validator domain.Validator) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		validator:        validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (container.ExecSession, domain.ValidationErrors, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}

	containers, err := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, request.UUID)
	if err != nil {
		return nil, nil, err
	}

	if len(containers) == 0 {
		return nil, nil, domain.ErrNotExists
	}

	running := containers[0]

	// whose it is, asked of the container itself. The node holding it wrote the
	// owner on when it created it, so it can answer this without a database and
	// without taking anybody's word for it.
	//
	// A container with no owner is a snippet: nobody's, and so everybody's. It
	// is a public playground, reachable by whoever finds it, and the terminal
	// is opened without asking who is knocking. An owner is what makes a
	// container private, and then it is private to exactly one person.
	if owner := running.Labels[container.TaskOwnerLabelKey]; len(owner) > 0 && owner != request.OwnerUUID {
		return nil, nil, domain.ErrNotExists
	}

	if running.Status != container.StatusRunning {
		return nil, domain.ValidationErrors{"uuid": "container_is_not_running"}, nil
	}

	session, err := uc.containerManager.Exec(ctx, running.ID, container.ExecOptions{
		Command: request.Shell(),
		TTY:     request.TTY,
	})
	if err != nil {
		return nil, nil, err
	}

	return session, nil, nil
}
