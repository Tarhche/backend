package restartTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// UseCase stops a task's container and starts it again in place, so it keeps
// its identity, its name and its log.
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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	containers, err := uc.containerManager.GetByLabel(ctx, container.TaskUUIDLabelKey, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, domain.ErrNotExists
	}

	for _, c := range containers {
		if err := uc.containerManager.Restart(ctx, c.ID); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
