package events

import "time"

const VMRestoredName = "workloadVMRestored"

// VMRestored says a VM's disk was replaced from a snapshot, so the restore it
// was waiting on is over.
type VMRestored struct {
	VMUUID       string    `json:"vm_uuid"`
	NodeName     string    `json:"node_name"`
	SnapshotUUID string    `json:"snapshot_uuid"`
	At           time.Time `json:"at"`
}
