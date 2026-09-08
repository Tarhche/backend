// Package watchStacks reports the stacks the runner is holding, so that whoever
// is showing them can be told when one changes instead of asking over and over.
//
// A stack's state is read off its services, so a stack changes whenever one of
// them does: it is reported whole, exactly as a stack asked for on its own is.
package watchStacks

import (
	"context"

	getstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Limit caps how many stacks one watch follows. A watch is what a listing is
// drawn from, and a listing is paginated, so this only has to cover what
// somebody could be looking at.
const Limit uint = 200

// Response is every stack the watch covers, as it is now.
type Response struct {
	Items []getstack.Response `json:"items"`
}

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

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	stacks, err := uc.stackRepository.GetAll(ctx, 0, Limit)
	if err != nil {
		return nil, err
	}

	items := make([]getstack.Response, len(stacks))
	for i, s := range stacks {
		services, err := uc.taskRepository.GetAllByStack(ctx, s.UUID)
		if err != nil {
			return nil, err
		}

		items[i] = *getstack.NewResponse(s, services)
	}

	return &Response{Items: items}, nil
}
