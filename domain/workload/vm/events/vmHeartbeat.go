package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const VMHeartbeatName = "workloadVMHeartbeat"

// VMHeartbeat is what a node says, every beat, about itself and the VMs it
// holds. A VM it does not list is one it does not have.
type VMHeartbeat struct {
	NodeName string `json:"node_name"`

	// Capacity is what the node offers to VMs, and how much of it is taken.
	Capacity Info `json:"capacity"`

	VMs []VMBeat  `json:"vms"`
	At  time.Time `json:"at"`
}

// VMBeat is one VM, as the engine holding it sees it.
type VMBeat struct {
	UUID string `json:"uuid"`

	// State is the engine's word for what the instance is doing. What that
	// makes the VM is the control plane's to decide, since it alone knows what
	// was asked of it.
	State vm.InstanceState `json:"state"`

	Reason    string    `json:"reason,omitempty"`
	Stats     Stats     `json:"stats"`
	StartedAt time.Time `json:"started_at"`
}
