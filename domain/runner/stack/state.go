package stack

import (
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// State reads a stack's condition off its services. A stack keeps no state of
// its own: it is exactly as running as the containers in it, so there is one
// state machine in the runner rather than two that can disagree.
func State(services []task.Task) task.State {
	if len(services) == 0 {
		return task.Failed
	}

	var (
		running    int
		terminal   int
		failed     int
		inProgress int
	)

	for _, service := range services {
		switch {
		case service.State == task.Failed:
			failed++
			terminal++
		case service.State == task.Running:
			running++
		case task.IsTerminalState(service.State):
			terminal++
		default:
			inProgress++
		}
	}

	switch {
	// anything still on its way makes the whole stack still on its way, so a
	// stack is only called running once nothing is left to start.
	case inProgress > 0:
		return task.Scheduled
	case running == len(services):
		return task.Running
	// a stack that lost a service is degraded, and the honest word for that is
	// the state of the service it lost.
	case failed > 0:
		return task.Failed
	case terminal == len(services):
		return task.Stopped
	default:
		return task.Scheduled
	}
}
