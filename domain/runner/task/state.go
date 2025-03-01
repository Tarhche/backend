package task

import (
	"slices"
)

// State represents the state of a task
type State int

func (s State) String() string {
	switch s {
	case Created:
		return "created"
	case Scheduled:
		return "scheduled"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	case Completed:
		return "completed"
	case Failed:
		return "failed"
	}

	return "unknown"
}

const (
	// Created is the state of a task that is created
	Created State = 1

	// Scheduled is the state of a task that is scheduled
	Scheduled State = 2

	// Running is the state of a task that is running
	Running State = 3

	// Stopping is the state of a task that is stopping
	Stopping State = 4

	// Stopped is the state of a task that is stopped
	Stopped State = 5

	// Completed is the state of a task that is completed
	Completed State = 6

	// Failed is the state of a task that is failed
	Failed State = 7
)

// stateTransitionMap is a map of state transitions
var stateTransitionMap = map[State][]State{
	Created:   {Scheduled},
	Scheduled: {Running, Stopping, Failed},
	Running:   {Stopping, Completed, Failed},
	Stopping:  {Stopped, Completed, Failed},
	Stopped:   {Scheduled},
	Completed: {Scheduled},
	Failed:    {Scheduled},
}

// terminalStates is a list of terminal states
var terminalStates = []State{
	Stopped,
	Completed,
	Failed,
}

// ValidStateTransition returns true if the state transition is valid
func ValidStateTransition(src State, dst State) bool {
	return slices.Contains(stateTransitionMap[src], dst)
}

// IsTerminalState returns true if the state is a terminal state
func IsTerminalState(state State) bool {
	return slices.Contains(terminalStates, state)
}
