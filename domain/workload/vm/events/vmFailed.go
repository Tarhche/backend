package events

import "time"

const VMFailedName = "workloadVMFailed"

// VMFailed says a node could not do what it was asked of a VM. Reason is why,
// in the engine's words.
type VMFailed struct {
	VMUUID   string    `json:"vm_uuid"`
	NodeName string    `json:"node_name"`
	Reason   string    `json:"reason"`
	At       time.Time `json:"at"`
}
