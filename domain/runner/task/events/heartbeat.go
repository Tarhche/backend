package events

import (
	"time"
)

const HeartbeatName = "runnerTaskHeartbeat"

type Heartbeat struct {
	UUID string
	Name string

	// Slug is the name the task's ports are served under, which is what
	// turns an exposed port into an address somebody can open.
	Slug string

	// Kind is what the task is running, so that whoever is listening can
	// tell a job it asked for from a service somebody else's dashboard did.
	Kind string

	// OwnerUUID is whose task this is, and StackUUID the stack it is a
	// service of, both read off the task itself. They travel with every
	// beat so that whoever is following the tasks can tell whose news
	// this is, and what else it changes, without asking anything.
	OwnerUUID string
	StackUUID string

	Image       string
	ExecutionID string
	State       int
	NodeName    string

	// Attempt is which try this task is, as it was created. A task
	// that fails carries the count of what came before it here.
	Attempt int

	// Interactive says this task is one somebody is watching while it
	// runs, rather than waiting on for what it prints.
	Interactive bool

	// Deadline is when the task will be stopped for having run long
	// enough, as it was labelled when it was made. A task that may run
	// for as long as it likes has none.
	Deadline time.Time

	Endpoints []Endpoint
	Logs      []byte
	At        time.Time
}
