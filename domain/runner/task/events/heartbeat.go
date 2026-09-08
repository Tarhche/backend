package events

import (
	"time"
)

const HeartbeatName = "runnerTaskHeartbeat"

type Heartbeat struct {
	UUID          string
	Name          string
	Image         string
	ContainerUUID string
	State         int
	NodeName      string
	Endpoints     []Endpoint
	Logs          []byte
	At            time.Time
}
