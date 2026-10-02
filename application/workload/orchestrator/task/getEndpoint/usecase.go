package getEndpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// UseCase says which of this node's runs a request to a task reaches, and on
// which of its ports, and connects to it there.
//
// Only the node holding a task can answer this: which of its ports can be
// reached is read back off the runtime every time, because a task that
// restarted is not reachable again until it is up, and one docker runs comes
// back on a published port of its own.
type UseCase struct {
	taskManager task.Runtime

	// advertiseHost is where this node reaches the ports docker published.
	// That is the docker daemon's own host, which is not always this one.
	advertiseHost string

	dialer net.Dialer
}

func NewUseCase(taskManager task.Runtime, advertiseHost string) *UseCase {
	return &UseCase{
		taskManager:   taskManager,
		advertiseHost: advertiseHost,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	runs, err := uc.taskManager.BySlug(ctx, request.Slug)
	if err != nil {
		return nil, err
	}

	if len(runs) == 0 {
		return nil, ErrNotHeld
	}

	run, found := serving(runs)
	if !found {
		return nil, ErrNotRunning
	}

	_, dialable := uc.taskManager.(task.Dialer)

	selected, found := selectPort(reachable(run, dialable), request.Port)
	if !found {
		return nil, ErrNotExposed
	}

	response := &Response{ExecutionID: run.ID, Port: selected}

	if published := publishedPort(run.PortBindings, selected); published > 0 {
		response.Host, response.HostPort = uc.advertiseHost, published
	}

	return response, nil
}

// Dial connects to the port a Response names.
//
// A runtime that reaches its runs itself is asked first. One that holds runs
// of several kinds may not be able to reach this one (task.ErrNotSupported),
// and a runtime that is no Dialer at all cannot reach any; either way the run
// is reached where its port was published, as it always was. The proxy is
// handed only the use case, which already holds the runtime, so this is where
// the dialling is.
//
// The context bounds connecting, not the connection.
func (uc *UseCase) Dial(ctx context.Context, endpoint *Response) (net.Conn, error) {
	if dialer, ok := uc.taskManager.(task.Dialer); ok {
		conn, err := dialer.DialContext(ctx, endpoint.ExecutionID, endpoint.Port)
		if !errors.Is(err, task.ErrNotSupported) {
			return conn, err
		}
	}

	if !endpoint.Published() {
		return nil, fmt.Errorf("%w: port %d of %s was published nowhere, and its runtime cannot reach it", ErrNotExposed, endpoint.Port, endpoint.ExecutionID)
	}

	return uc.dialer.DialContext(ctx, "tcp", endpoint.Address())
}

// serving is the run a request to a task reaches: the latest of its runs that
// is running. A task being retried has a run on its way out beside the one on
// its way in, and whichever of them a runtime happens to list last, it is the
// one that runs that can answer.
func serving(runs []task.Execution) (task.Execution, bool) {
	var latest task.Execution
	var found bool

	for _, run := range runs {
		if task.EvaluateState(run.Status, run.Kind, run.ExitCode) != task.Running {
			continue
		}

		if !found || !run.CreatedAt.Before(latest.CreatedAt) {
			latest, found = run, true
		}
	}

	return latest, found
}

// reachable is the ports a request can reach a run on right now.
//
// A runtime says which of a run's ports it can reach (Execution.Endpoints).
// One that does not say is read the way docker always was: a port is reachable
// where docker published it. And a runtime that cannot dial reaches nothing it
// did not publish, whatever it says.
func reachable(run task.Execution, dialable bool) []port.Port {
	if len(run.Endpoints) == 0 {
		return publishedPorts(run.PortBindings)
	}

	if dialable {
		return run.Endpoints
	}

	ports := make([]port.Port, 0, len(run.Endpoints))
	for _, p := range run.Endpoints {
		if publishedPort(run.PortBindings, p) > 0 {
			ports = append(ports, p)
		}
	}

	return ports
}

// publishedPorts is every port of a run that was published somewhere.
func publishedPorts(bindings port.PortMap) []port.Port {
	ports := make([]port.Port, 0, len(bindings))
	for taskPort := range bindings {
		if publishedPort(bindings, taskPort) > 0 {
			ports = append(ports, taskPort)
		}
	}

	return ports
}

// publishedPort is where one of a run's ports was published, or zero when it
// was not.
func publishedPort(bindings port.PortMap, taskPort port.Port) port.Port {
	published := bindings[taskPort]
	if len(published) == 0 {
		return 0
	}

	return published[0].HostPort
}

// selectPort picks the port a hostname asked for. A hostname naming no port
// reaches the lowest one the task can be reached on, so the common case of a
// task with a single port needs no port in its name at all.
func selectPort(ports []port.Port, requested port.Port) (port.Port, bool) {
	if len(ports) == 0 {
		return 0, false
	}

	if requested > 0 {
		return requested, slices.Contains(ports, requested)
	}

	// a runtime hands them back in no particular order, and the lowest one is
	// the one a bare hostname reaches.
	return slices.Min(ports), true
}
