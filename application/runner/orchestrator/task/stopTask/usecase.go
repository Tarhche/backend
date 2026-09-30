package stopTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// StopTask stops a task
type UseCase struct {
	taskManager task.Runtime
	validator   domain.Validator
}

// NewUseCase creates a new UseCase
func NewUseCase(
	taskManager task.Runtime,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		taskManager: taskManager,
		validator:   validator,
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
		err := uc.taskManager.Stop(ctx, c.ID)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
