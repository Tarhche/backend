package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/stats"
)

const HeartbeatName = "runnerNodeHeartbeat"

type Heartbeat struct {
	Name      string
	Role      node.Role
	Resources node.Resource
	Stats     stats.Stats
	At        time.Time
}
