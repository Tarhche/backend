package events

import "time"

const TaskRanName = "runnerTaskRan"

type TaskRan struct {
	UUID          string     `json:"uuid"`
	NodeName      string     `json:"node_name"`
	ContainerUUID string     `json:"container_uuid"`
	Endpoints     []Endpoint `json:"endpoints"`
	StartedAt     time.Time  `json:"started_at"`

	// Deadline is when the container will be stopped for having run long
	// enough. It is set as the container is made, so what it may run for is
	// counted from when it came up.
	Deadline time.Time `json:"deadline,omitempty"`
}
