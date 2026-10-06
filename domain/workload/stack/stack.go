// Package stack is a compose project deployed into a Docker VM, as the blog
// shows one.
//
// What a stack is, and everything the workload does with it, is the stack
// kind's (domain/workload/kinds/stack): the control plane keeps it as a
// manifest. This is the shape the blog's dashboard reads one in, which the
// control plane's client reads a manifest back as, so that the dashboard's
// answers stay what they always were.
package stack

import "time"

// Stack is one compose project.
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

	// Slug is the compose project's name: unique, lowercase letters, digits
	// and dashes. What the compose file calls its project is ignored.
	Slug string

	// Compose is the YAML as it was given. A stack is not edited: there is no
	// update, only deploy and remove.
	Compose string

	// ExpectedState is what the stack was asked to be, Running or Stopped.
	// State is what it was last seen doing.
	ExpectedState State
	State         State

	Reason string

	// Output is the tail of what the last command run on it printed, which
	// is how somebody finds out why a service did not come up.
	Output string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// State is where a stack is in its life: one of the stack kind's states.
type State int

const (
	// Deploying is a stack whose project is being brought up.
	Deploying State = 1

	// Running is a stack whose every service runs.
	Running State = 2

	Starting State = 3
	Stopping State = 4

	// Stopped is a stack whose containers are stopped and kept.
	Stopped State = 5

	Restarting State = 6

	// Removing is a stack whose project is being taken down. Its record goes
	// once that is done.
	Removing State = 7

	// Failed is a stack whose last command failed. Its Reason and Output say
	// why.
	Failed State = 8

	// Degraded is a stack some of whose services are not running. It is
	// deployed again, to bring them back.
	Degraded State = 9

	// Waiting is a stack that is not in its VM: one whose VM is not running,
	// and one not deployed there yet, or any more. Its Reason says which.
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
