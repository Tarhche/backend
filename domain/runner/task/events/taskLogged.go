package events

import "time"

const TaskLoggedName = "runnerTaskLogged"

// TaskLogged carries a batch of lines a container wrote. The worker ships them
// as they are produced and the manager is what stores them, so a line survives
// its container.
type TaskLogged struct {
	UUID          string    `json:"uuid"`
	ContainerUUID string    `json:"container_uuid"`
	NodeName      string    `json:"node_name"`
	Lines         []LogLine `json:"lines"`
}

type LogLine struct {
	Stream  uint8     `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
