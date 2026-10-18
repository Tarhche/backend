// Package stack is a stack in a Docker VM, as the blog shows one: the
// services, volumes and networks of its file, which its graph made there.
//
// What a stack is, and everything the workload does with it, is the stack
// kind's (domain/workload/kinds/stack): the control plane keeps it as a
// manifest. This is the shape the blog's dashboard reads one in, which the
// control plane's client reads a manifest back as, so that the dashboard's
// answers stay what they always were.
package stack

import "time"

// Stack is one stack.
type Stack struct {
	UUID      string
	Name      string
	OwnerUUID string

	// VMUUID is the Docker VM the stack is deployed into.
	VMUUID string

	// VMName is what that VM is called now. It is not kept with the stack:
	// it is read with it, from the VM, so a VM that is renamed is named anew
	// wherever its stacks are shown.
	VMName string

	// Slug is what it is known by inside its VM: unique, lowercase letters,
	// digits and dashes. What its graph makes is named under it,
	// <slug>_<name>.
	Slug string

	// Compose is its file, the YAML as it was given. A stack is not edited:
	// there is no update, only deploy and remove.
	Compose string

	// ExpectedState is what the stack was asked to be, Running or Stopped.
	// State is what it was last seen doing.
	ExpectedState State
	State         State

	// Reason is why it is in its state, when its state says nothing alone:
	// what it waits on, or what its plan failed with.
	Reason string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// State is where a stack is in its life: one of the stack kind's states.
type State int

const (
	// Deploying is a stack its graph is making.
	Deploying State = 1

	// Running is a stack whose every service runs.
	Running State = 2

	Starting State = 3
	Stopping State = 4

	// Stopped is a stack whose containers are stopped and kept.
	Stopped State = 5

	Restarting State = 6

	// Removing is a stack its graph is removing. Its record goes once that is
	// done.
	Removing State = 7

	// Failed is a stack whose plan failed. Its Reason says why.
	Failed State = 8

	// Degraded is a stack some of whose services are not running: one whose
	// container fell over stays down, as a container of its own would, until
	// the stack is started.
	Degraded State = 9

	// Waiting is a stack whose graph waits on what it needs, which cannot be
	// made now: its VM is not running, or a container of it is being removed.
	// Its Reason says which.
	Waiting State = 10
)

// words are the states as the workload names them.
var words = map[State]string{
	Deploying:  "deploying",
	Running:    "running",
	Starting:   "starting",
	Stopping:   "stopping",
	Stopped:    "stopped",
	Restarting: "restarting",
	Removing:   "removing",
	Failed:     "failed",
	Degraded:   "degraded",
	Waiting:    "waiting",
}

func (s State) String() string {
	if word, known := words[s]; known {
		return word
	}

	return "unknown"
}

// StateOf is the state a word names, and none for a word that names none.
func StateOf(word string) State {
	for state, named := range words {
		if named == word {
			return state
		}
	}

	return 0
}
