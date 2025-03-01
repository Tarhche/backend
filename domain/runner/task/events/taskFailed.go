package events

import "time"

const TaskFailedName = "runnerTaskFailed"

type TaskFailed struct {
	UUID          string    `json:"uuid"`
	ContainerUUID string    `json:"container_uuid"`
	NodeName      string    `json:"node_name"`
	FailedAt      time.Time `json:"failed_at"`
}
