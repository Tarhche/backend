package events

import (
	"time"
)

const HeartbeatName = "workloadTaskHeartbeat"

type Heartbeat struct {
	UUID string
	Name string

	// Slug is the name the task's ports are served under, which is what
	// turns an exposed port into an address somebody can open.
	Slug string

	// Kind is what the task is running, so that whoever is listening can
	// tell a job it asked for from a service somebody else's dashboard did.
	Kind string

	// OwnerUUID is whose task this is, read off the task itself. It
	// travels with every beat so that whoever is following the tasks can
	// tell whose news this is without asking anything.
	OwnerUUID string

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
