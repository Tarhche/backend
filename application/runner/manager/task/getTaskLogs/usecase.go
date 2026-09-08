package getTaskLogs

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// UseCase reads what a container has written. The lines are kept against the
// task, so they go back to the container's first line and stay readable after
// it has stopped — until the task itself is deleted.
type UseCase struct {
	logRepository container.LogRepository
	validator     domain.Validator
}

func NewUseCase(logRepository container.LogRepository, validator domain.Validator) *UseCase {
	return &UseCase{
		logRepository: logRepository,
		validator:     validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	logs, err := uc.logRepository.Get(ctx, request.UUID, request.After, request.Limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(logs), nil
}
