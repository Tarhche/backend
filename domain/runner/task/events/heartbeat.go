package events

import (
	"time"
)

const HeartbeatName = "runnerTaskHeartbeat"

type Heartbeat struct {
	UUID string
	Name string

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

	Endpoints []Endpoint
	Logs      []byte
	At        time.Time
}
