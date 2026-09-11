package events

import (
	"time"
)

const HeartbeatName = "runnerTaskHeartbeat"

type Heartbeat struct {
	UUID string
	Name string

	// Slug is the name the container's ports are served under, which is what
	// turns an exposed port into an address somebody can open.
	Slug string

	// Kind is what the container is running, so that whoever is listening can
	// tell a job it asked for from a service somebody else's dashboard did.
	Kind string

	Image         string
	ContainerUUID string
	State         int
	NodeName      string

	// Attempt is which try this container is, as it was created. A container
	// that fails carries the count of what came before it here.
	Attempt int

	// Interactive says this container is one somebody is watching while it
	// runs, rather than waiting on for what it prints.
	Interactive bool

	// Deadline is when the container will be stopped for having run long
	// enough, as it was labelled when it was made. A container that may run
	// for as long as it likes has none.
	Deadline time.Time

	Endpoints []Endpoint
	Logs      []byte
	At        time.Time
}
