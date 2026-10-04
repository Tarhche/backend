package vm

import (
	"slices"
)

// State is where a VM is in its life.
type State int

const (
	// Created is a VM that has been asked for and not yet placed on a node.
	Created State = 1

	// Scheduled is a VM a node has been asked to create and boot.
	Scheduled State = 2

	// Starting is a stopped VM a node has been asked to boot again.
	Starting State = 3

	// Running is a VM that is up.
	Running State = 4

	// Stopping is a VM a node has been asked to stop.
	Stopping State = 5

	// Stopped is a VM that is down, with its disk kept where it lives.
	Stopped State = 6

	// Restarting is a VM being stopped and booted again in place, which is
	// also how a change to its ports, network or resources is applied.
	Restarting State = 7

	// Restoring is a VM whose disk is being replaced from a snapshot. It keeps
	// its identity: its uuid, its slug and its ports.
	Restoring State = 8

	// Failed is a VM that could not be made what it was asked to be. Its
	// Reason says why, when the workload can tell.
	Failed State = 9

	// Deleting is a VM a node has been asked to remove. Its record goes once
	// the node confirms it is gone.
	Deleting State = 10
)

func (s State) String() string {
	switch s {
	case Created:
		return "created"
	case Scheduled:
		return "scheduled"
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	case Restarting:
		return "restarting"
	case Restoring:
		return "restoring"
	case Failed:
		return "failed"
	case Deleting:
		return "deleting"
	}

	return "unknown"
}

// stateTransitionMap is where a VM may be asked to go from where it is.
//
// Deleting leads nowhere: a VM on its way out is not brought back, and its
// record goes once its node says it is gone. Every other state may be asked
// to delete, and so may a failed VM be asked for again, in place or anew.
var stateTransitionMap = map[State][]State{
	Created:    {Scheduled, Failed, Deleting},
	Scheduled:  {Starting, Running, Stopping, Failed, Deleting},
	Starting:   {Running, Stopping, Failed, Deleting},
	Running:    {Stopping, Stopped, Restarting, Restoring, Failed, Deleting},
	Stopping:   {Stopped, Failed, Deleting},
	Stopped:    {Scheduled, Starting, Restoring, Failed, Deleting},
	Restarting: {Running, Stopping, Failed, Deleting},
	Restoring:  {Running, Stopped, Failed, Deleting},
	Failed:     {Scheduled, Starting, Restarting, Restoring, Deleting},
}

// ValidStateTransition reports whether a VM in src may be asked to go to dst.
func ValidStateTransition(src State, dst State) bool {
	return slices.Contains(stateTransitionMap[src], dst)
}

// terminalStates are the states a VM rests in when it is not running: it has
// ended, one way or the other, and nothing is under way.
var terminalStates = []State{
	Stopped,
	Failed,
}

// IsTerminalState reports whether a VM in this state has ended.
func IsTerminalState(state State) bool {
	return slices.Contains(terminalStates, state)
}

// inFlightStates are the states a VM is passing through rather than resting
// in: something has been asked of it and has yet to happen.
var inFlightStates = []State{
	Created,
	Scheduled,
	Starting,
	Stopping,
	Restarting,
	Restoring,
	Deleting,
}

// IsInFlightState reports whether a VM is on its way somewhere, which is not
// the same as being somewhere it should not be.
func IsInFlightState(state State) bool {
	return slices.Contains(inFlightStates, state)
}
