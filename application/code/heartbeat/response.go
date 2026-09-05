package heartbeat

import "time"

type Response struct {
	Name string `json:"name"`
	Logs []byte `json:"logs"`

	// State is what the container running the code is doing, and Endpoints
	// where it can be reached while it is running. A snippet that serves
	// nothing has none, and one that has stopped has none any more.
	State     string     `json:"state,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	// Deadline is when a snippet somebody is watching will be stopped, which
	// is what the countdown beside it counts down to. A snippet nobody is
	// watching is answered once and has none.
	Deadline *time.Time `json:"deadline,omitempty"`

	// ContainerUUID is the container itself, which is what a terminal is
	// opened on. It is known only to whoever ran the code.
	ContainerUUID string `json:"container_uuid,omitempty"`

	// Error is why the code never ran, when it never did. Code that ran says
	// what it has to say through its output, however badly it went.
	Error string `json:"error,omitempty"`
}

// Endpoint is one of the addresses a running snippet answers on.
type Endpoint struct {
	ContainerPort uint   `json:"container_port"`
	URL           string `json:"url"`
}
