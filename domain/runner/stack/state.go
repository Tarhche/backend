package stack

import (
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// State reads a stack's condition off its services. A stack keeps no state of
// its own: it is exactly as running as the containers in it, so there is one
// state machine in the runner rather than two that can disagree.
// ExpectedState is what a stack was asked to be, read off what its services
// were asked to be.
//
// A stack is asked for as a whole, so its services agree; while a command is
// still reaching all of them they may not, and one service still wanted running
// is a stack still wanted running.
func ExpectedState(services []task.Task) task.State {
	var running, stopped, failed int

	for _, service := range services {
		switch service.ExpectedState {
		case task.Running:
			running++
		case task.Stopped:
			stopped++
		case task.Failed:
			failed++
		}
	}

	switch {
	case running > 0:
		return task.Running
	case stopped > 0:
		return task.Stopped
	case failed > 0:
		return task.Failed
	default:
		// a stack from before there were expectations was not asked for
		// anything in particular.
		return 0
	}
}

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
		case service.CurrentState == task.Failed:
			failed++
			terminal++
		case service.CurrentState == task.Running:
			running++
		case task.IsTerminalState(service.CurrentState):
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
