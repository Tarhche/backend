package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

const HeartbeatName = "runnerNodeHeartbeat"

type Heartbeat struct {
	Name  string
	Role  node.Role
	Stats node.Stats
	At    time.Time
}
