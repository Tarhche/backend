package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

const HeartbeatName = "workloadNodeHeartbeat"

type Heartbeat struct {
	Name  string
	Role  node.Role
	Stats node.Stats

	// Runtimes are the classes the node offers, one offer each. An offer
	// that is not healthy is still there, so that what the node runs under
	// it is taken for unknown while its driver is away rather than for lost.
	Runtimes []runtime.Offer `json:",omitempty"`

	At time.Time
}
