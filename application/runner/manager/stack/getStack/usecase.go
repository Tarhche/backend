package getStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type UseCase struct {
	stackRepository stack.Repository
	taskRepository  task.Repository
}

func NewUseCase(stackRepository stack.Repository, taskRepository task.Repository) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		taskRepository:  taskRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, UUID string) (*Response, error) {
	s, err := uc.stackRepository.GetOne(ctx, UUID)
	if err != nil {
		return nil, err
	}

	services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(s, services), nil
}
