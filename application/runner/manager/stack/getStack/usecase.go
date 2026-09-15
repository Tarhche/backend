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

	return uc.with(ctx, s)
}

// ExecuteOwn is the same, of one person's own stack. One that is not theirs is
// not there for them, which is what the caller passes on.
func (uc *UseCase) ExecuteOwn(ctx context.Context, ownerUUID string, UUID string) (*Response, error) {
	s, err := uc.stackRepository.GetOneByOwner(ctx, ownerUUID, UUID)
	if err != nil {
		return nil, err
	}

	return uc.with(ctx, s)
}

func (uc *UseCase) with(ctx context.Context, s stack.Stack) (*Response, error) {
	services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(s, services), nil
}
