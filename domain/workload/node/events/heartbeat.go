package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const HeartbeatName = "workloadNodeHeartbeat"

// Heartbeat is what a node says of itself every beat: that it is alive, what
// it is, what it uses and what it offers. What every kind it runs holds on it
// is said in a heartbeat of the kind's own (kind.Heartbeat), stamped as this
// one is.
type Heartbeat struct {
	Name  string
	Role  node.Role
	Stats node.Stats
	At    time.Time

	// Capacity is what the node offers to VMs and how much of it is taken,
	// which is what VMs are placed by. A node that could not say sends none,
	// as every heartbeat did before VMs were a kind.
	Capacity vm.Info `json:",omitzero"`
}
