package gettask

import "github.com/khanzadimahdi/testproject/domain/runner/task"

// UseCase retrieves a task
type UseCase struct {
	taskRepository task.Repository
}

// NewUseCase creates an instance of the UseCase
func NewUseCase(taskRepository task.Repository) *UseCase {
	return &UseCase{
		taskRepository: taskRepository,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(UUID string) (*Response, error) {
	t, err := uc.taskRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(t), nil
}
