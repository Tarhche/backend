package events

import "time"

const TaskRanName = "runnerTaskRan"

type TaskRan struct {
	UUID          string     `json:"uuid"`
	NodeName      string     `json:"node_name"`
	ContainerUUID string     `json:"container_uuid"`
	Endpoints     []Endpoint `json:"endpoints"`
	StartedAt     time.Time  `json:"started_at"`
}
