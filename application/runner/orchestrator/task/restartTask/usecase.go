package restartTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase stops a task's task and starts it again in place, so it keeps
// its identity, its name and its log.
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
		if err := uc.taskManager.Restart(ctx, c.ID); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
