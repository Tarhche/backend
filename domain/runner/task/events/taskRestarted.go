package events

import "time"

const TaskRestartedName = "runnerTaskRestarted"

type TaskRestarted struct {
	UUID          string    `json:"uuid"`
	NodeName      string    `json:"node_name"`
	ContainerUUID string    `json:"container_uuid"`
	At            time.Time `json:"at"`
}
