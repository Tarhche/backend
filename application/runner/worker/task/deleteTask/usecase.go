package deleteTask

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// UseCase takes a container away.
//
// What is still running is stopped first and only then removed, so a container
// ends the way it would if it had been stopped: its process is asked to finish
// rather than pulled out from under itself.
type UseCase struct {
	containerManager container.Manager
	validator        domain.Validator
	logger           *slog.Logger
}

// NewUseCase creates a new UseCase
func NewUseCase(
	containerManager container.Manager,
	validator domain.Validator,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		validator:        validator,
		logger:           logger,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	containers, err := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, domain.ErrNotExists
	}

	for _, c := range containers {
		if c.Status == container.StatusRunning {
			// a container that will not stop is still one to take away, so its
			// refusal is noted rather than obeyed: the removal below is forced.
			if err := uc.containerManager.Stop(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
				uc.logger.WarnContext(ctx, "a container would not stop before being removed", "error", err, "container", c.ID)
			}
		}

		if err := uc.containerManager.Delete(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return nil, err
		}
	}

	return nil, nil
}
