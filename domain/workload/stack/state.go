package stack

import "slices"

// State is where a stack is in its life.
type State int

const (
	// Deploying is a stack whose project is being brought up.
	Deploying State = 1

	// Running is a stack whose last compose command brought it up.
	Running State = 2

	Starting State = 3
	Stopping State = 4

	// Stopped is a stack whose containers are stopped and kept.
	Stopped State = 5

	Restarting State = 6

	// Removing is a stack whose project is being taken down. Its record goes
	// once that is done.
	Removing State = 7

	// Failed is a stack whose last compose command failed. Its Reason and
	// Output say why.
	Failed State = 8
)

func (s State) String() string {
	switch s {
	case Deploying:
		return "deploying"
	case Running:
		return "running"
	case Starting:
		return "starting"
	case Stopping:
		return "stopping"
	case Stopped:
		return "stopped"
	case Restarting:
		return "restarting"
	case Removing:
		return "removing"
	case Failed:
		return "failed"
	}

	return "unknown"
}

// stateTransitionMap is where a stack may be asked to go from where it is.
//
// A stack is deployed once, when it is created; after that it is started,
// stopped and restarted. Removing leads nowhere but to a failure to remove: a
// stack on its way out is not brought back. Every other state may be asked to
// remove, and a failed stack may be asked for anything again, a deploy
// included.
var stateTransitionMap = map[State][]State{
	Deploying:  {Running, Failed, Removing},
	Running:    {Stopping, Restarting, Failed, Removing},
	Starting:   {Running, Failed, Removing},
	Stopping:   {Stopped, Failed, Removing},
	Stopped:    {Starting, Failed, Removing},
	Restarting: {Running, Failed, Removing},
	Removing:   {Failed},
	Failed:     {Deploying, Starting, Stopping, Restarting, Removing},
}

// ValidStateTransition reports whether a stack in src may be asked to go to
// dst.
func ValidStateTransition(src State, dst State) bool {
	return slices.Contains(stateTransitionMap[src], dst)
}

// inFlightStates are the states a stack is in while a compose command runs.
var inFlightStates = []State{
	Deploying,
	Starting,
	Stopping,
	Restarting,
	Removing,
}

// IsInFlightState reports whether a compose command is under way on a stack
// in this state.
func IsInFlightState(state State) bool {
	return slices.Contains(inFlightStates, state)
}
