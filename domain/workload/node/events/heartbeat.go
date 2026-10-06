package events

import (
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const HeartbeatName = "workloadNodeHeartbeat"

type Heartbeat struct {
	Name  string
	Role  node.Role
	Stats node.Stats
	At    time.Time

	// Capacity is what the node offers to VMs and how much of it is taken,
	// which is what VMs are placed by. A node that could not say sends none,
	// as every heartbeat did before VMs were a kind.
	Capacity vm.Info `json:",omitzero"`

	// Observations are what the state action of every kind this node runs
	// found on it this beat, by kind.
	//
	// A kind that is here reports everything of it the node holds: what it
	// does not list is not on the node, except inside the parents its report
	// could not look into. A kind that is not here could not say anything
	// this beat, and nothing is concluded from its silence. A node that runs
	// no kinds yet sends none, which is every heartbeat that was sent before
	// kinds were.
	Observations map[string]kind.Report[json.RawMessage] `json:"observations,omitempty"`
}
