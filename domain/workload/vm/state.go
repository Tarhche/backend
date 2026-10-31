package vm

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
