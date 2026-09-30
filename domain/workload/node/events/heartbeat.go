package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
)

const HeartbeatName = "workloadNodeHeartbeat"

type Heartbeat struct {
	Name  string
	Role  node.Role
	Stats node.Stats
	At    time.Time
}
