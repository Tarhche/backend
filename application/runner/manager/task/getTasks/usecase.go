package gettasks

import (
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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalTasks, err := uc.taskRepository.Count()
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

	tasks, err := uc.taskRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(tasks, totalPages, currentPage), nil
}
