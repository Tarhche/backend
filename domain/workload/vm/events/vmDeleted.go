package events

import "time"

const VMDeletedName = "workloadVMDeleted"

// VMDeleted says a node no longer holds a VM it was asked to remove, so its
// record can go without waiting for a heartbeat to leave it out.
type VMDeleted struct {
	VMUUID   string    `json:"vm_uuid"`
	NodeName string    `json:"node_name"`
	At       time.Time `json:"at"`
}
