// Package watchTasks reports the containers the runner is holding, so that
// whoever is showing them can be told when one changes instead of asking over
// and over.
package watchTasks

import (
	"context"

	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Limit caps how many containers one watch follows. A watch is what a listing
// is drawn from, and a listing is paginated, so this only has to cover what
// somebody could be looking at.
const Limit uint = 200

// Response is every container the watch covers, as it is now.
type Response struct {
	Items []gettask.Response `json:"items"`
}

type UseCase struct {
	taskRepository task.Repository
}

func NewUseCase(taskRepository task.Repository) *UseCase {
	return &UseCase{taskRepository: taskRepository}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	tasks, err := uc.taskRepository.GetAll(ctx, 0, Limit)
	if err != nil {
		return nil, err
	}

	items := make([]gettask.Response, len(tasks))
	for i, t := range tasks {
		items[i] = *gettask.NewResponse(t)
	}

	return &Response{Items: items}, nil
}
