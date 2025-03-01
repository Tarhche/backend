package stopTask

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// StopTask stops a task
type UseCase struct {
	containerManager container.Manager
	validator        domain.Validator
}

// NewUseCase creates a new UseCase
func NewUseCase(
	containerManager container.Manager,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		validator:        validator,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	containers, err := uc.containerManager.GetByLabel(container.TaskUUIDLabelKey, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, domain.ErrNotExists
	}

	for _, c := range containers {
		err := uc.containerManager.Stop(c.ID)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
