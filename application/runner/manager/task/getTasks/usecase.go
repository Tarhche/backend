package gettasks

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const limit = 10

type UseCase struct {
	taskRepository task.Repository
}

func NewUseCase(taskRepository task.Repository) *UseCase {
	return &UseCase{
		taskRepository: taskRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalTasks, err := uc.count(ctx, request)
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalTasks / limit

	if (totalPages * limit) != totalTasks {
		totalPages++
	}

	tasks, err := uc.page(ctx, request, offset)
	if err != nil {
		return nil, err
	}

	return NewResponse(tasks, totalPages, currentPage), nil
}

// count is how many containers the listing covers: everybody's, or one
// person's.
func (uc *UseCase) count(ctx context.Context, request *Request) (uint, error) {
	if len(request.OwnerUUID) == 0 {
		return uc.taskRepository.Count(ctx)
	}

	return uc.taskRepository.CountByOwner(ctx, request.OwnerUUID)
}

// page is the containers themselves, of the same ones count counted.
func (uc *UseCase) page(ctx context.Context, request *Request, offset uint) ([]task.Task, error) {
	if len(request.OwnerUUID) == 0 {
		return uc.taskRepository.GetAll(ctx, offset, limit)
	}

	return uc.taskRepository.GetAllByOwner(ctx, request.OwnerUUID, offset, limit)
}
