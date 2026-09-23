package getEndpoint

import (
	"context"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase says which of this node's tasks a request reaches, and on which
// port.
//
// Only the node holding a task can answer this: which of its ports can be
// reached is read back off the runtime every time, because a task that
// restarted is not reachable again until it is up.
type UseCase struct {
	taskManager task.Runtime
}

func NewUseCase(taskManager task.Runtime) *UseCase {
	return &UseCase{
		taskManager: taskManager,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	tasks, err := uc.taskManager.BySlug(ctx, request.Slug)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, ErrNotHeld
	}

	c := tasks[len(tasks)-1]

	if task.EvaluateState(c.Status, c.Kind, c.ExitCode) != task.Running {
		return nil, ErrNotRunning
	}

	selected, found := selectPort(c.Endpoints, request.Port)
	if !found {
		return nil, ErrNotExposed
	}

	return &Response{ExecutionID: c.ID, Port: selected}, nil
}

// selectPort picks the port a hostname asked for. A hostname naming no port
// reaches the lowest one the task can be reached on, so the common case of a
// task with a single port needs no port in its name at all.
func selectPort(endpoints []port.Port, requested port.Port) (port.Port, bool) {
	if len(endpoints) == 0 {
		return 0, false
	}

	if requested > 0 {
		return requested, slices.Contains(endpoints, requested)
	}

	// a runtime hands them back in no particular order, and the lowest one is
	// the one a bare hostname reaches.
	return slices.Min(endpoints), true
}
