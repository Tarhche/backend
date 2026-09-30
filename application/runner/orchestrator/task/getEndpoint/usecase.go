package getEndpoint

import (
	"context"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase says where one of this node's tasks can be reached.
//
// Only the node holding a task can answer this: the port docker published
// it on is read back off docker every time, because a task that restarted
// came back on a different one.
type UseCase struct {
	taskManager task.Runtime

	// advertiseHost is where this node reaches the ports its tasks were
	// published on. That is the docker daemon's own host, which is not always
	// this one.
	advertiseHost string
}

func NewUseCase(taskManager task.Runtime, advertiseHost string) *UseCase {
	return &UseCase{
		taskManager:   taskManager,
		advertiseHost: advertiseHost,
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

	published, found := selectPort(c.PortBindings, request.Port)
	if !found {
		return nil, ErrNotExposed
	}

	return &Response{Host: uc.advertiseHost, Port: published}, nil
}

// selectPort picks the published port a hostname asked for. A hostname naming
// no port reaches the lowest one the task exposes, so the common case of a
// task with a single port needs no port in its name at all.
func selectPort(bindings port.PortMap, requested port.Port) (port.Port, bool) {
	exposed := make([]port.Port, 0, len(bindings))
	for taskPort, published := range bindings {
		if len(published) == 0 || published[0].HostPort == 0 {
			continue
		}

		if requested > 0 && taskPort != requested {
			continue
		}

		exposed = append(exposed, taskPort)
	}

	if len(exposed) == 0 {
		return 0, false
	}

	// docker hands the bindings back in no particular order, and the lowest
	// exposed port is the one a bare hostname reaches.
	lowest := slices.Min(exposed)

	return bindings[lowest][0].HostPort, true
}
