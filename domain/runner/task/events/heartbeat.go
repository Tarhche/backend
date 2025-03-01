package events

import (
	"time"
)

const HeartbeatName = "task.heartbeat"

type Heartbeat struct {
	UUID          string
	Name          string
	Image         string
	ContainerUUID string
	State         int
	NodeName      string
	Logs          []byte
	At            time.Time
}
