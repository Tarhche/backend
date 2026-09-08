package getStacks

import (
	"context"
	"math"

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if request.Page == 0 {
		request.Page = 1
	}

	total, err := uc.stackRepository.Count(ctx)
	if err != nil {
		return nil, err
	}

	totalPages := uint(math.Ceil(float64(total) / float64(Limit)))

	stacks, err := uc.stackRepository.GetAll(ctx, (request.Page-1)*Limit, Limit)
	if err != nil {
		return nil, err
	}

	// a stack's state is read off its services, so the listing needs them too.
	services := make(map[string][]task.Task, len(stacks))
	for _, s := range stacks {
		items, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
		if err != nil {
			return nil, err
		}

		services[s.UUID] = items
	}

	return NewResponse(stacks, services, totalPages, request.Page), nil
}
