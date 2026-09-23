package deleteTask

import (
	"context"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase takes a task away.
//
// What is still running is stopped first and only then removed, so a task
// ends the way it would if it had been stopped: its process is asked to finish
// rather than pulled out from under itself.
type UseCase struct {
	taskManager task.Runtime
	validator   domain.Validator
	logger      *slog.Logger
}

// NewUseCase creates a new UseCase
func NewUseCase(
	taskManager task.Runtime,
	validator domain.Validator,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		taskManager: taskManager,
		validator:   validator,
		logger:      logger,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	tasks, err := uc.taskManager.Of(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, domain.ErrNotExists
	}

	for _, c := range tasks {
		if c.Status == task.StatusRunning {
			// a task that will not stop is still one to take away, so its
			// refusal is noted rather than obeyed: the removal below is forced.
			if err := uc.taskManager.Stop(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
				uc.logger.WarnContext(ctx, "a task would not stop before being removed", "error", err, "task", c.ID)
			}
		}

		if err := uc.taskManager.Delete(ctx, c.ID); err != nil && !errors.Is(err, domain.ErrNotExists) {
			return nil, err
		}
	}

	return nil, nil
}
