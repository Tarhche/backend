package killTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase stops a task's task at once, without the grace period a stop
// gives it.
type UseCase struct {
	taskManager task.Runtime
	validator   domain.Validator
}

func NewUseCase(taskManager task.Runtime, validator domain.Validator) *UseCase {
	return &UseCase{
		taskManager: taskManager,
		validator:   validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	tasks, err := uc.taskManager.Of(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, domain.ErrNotExists
	}

	for _, c := range tasks {
		if err := uc.taskManager.Kill(ctx, c.ID); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
