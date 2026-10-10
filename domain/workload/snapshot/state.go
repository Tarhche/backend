package snapshot

// State is where a snapshot is in its life, as the dashboard has always
// named it: one of the snapshot kind's states.
type State int

const (
	// Creating is a snapshot its VM's node has been asked to take and store.
	Creating State = 1

	// Ready is a snapshot that is stored and can be restored.
	Ready State = 2

	// Failed is a snapshot that could not be taken or stored. Its Reason
	// says why.
	Failed State = 3

	// Deleting is a snapshot on its way out: its archive is being taken
	// away, or is to be once its node has finished taking it.
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
