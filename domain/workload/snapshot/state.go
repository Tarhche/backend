package snapshot

import "slices"

// State is where a snapshot is in its life.
type State int

const (
	// Creating is a snapshot its VM's node has been asked to take and store.
	Creating State = 1

	// Ready is a snapshot that is stored and can be restored.
	Ready State = 2

	// Failed is a snapshot that could not be taken or stored. Its Reason
	// says why.
	Failed State = 3

	// Deleting is a snapshot whose archive is being taken away.
	Deleting State = 4
)

func (s State) String() string {
	switch s {
	case Creating:
		return "creating"
	case Ready:
		return "ready"
	case Failed:
		return "failed"
	case Deleting:
		return "deleting"
	}

	return "unknown"
}

// stateTransitionMap is where a snapshot may go from where it is. A snapshot is
// never taken again: one that failed is deleted, and another is asked for.
var stateTransitionMap = map[State][]State{
	Creating: {Ready, Failed, Deleting},
	Ready:    {Deleting},
	Failed:   {Deleting},
}

// ValidStateTransition reports whether a snapshot in src may go to dst.
func ValidStateTransition(src State, dst State) bool {
	return slices.Contains(stateTransitionMap[src], dst)
}
