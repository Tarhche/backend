// Package events is what the control plane and the nodes tell each other about
// snapshots, over JetStream.
package events

const SnapshotRequestedName = "workloadSnapshotRequested"

// SnapshotRequested asks the node holding a VM to take a snapshot of its disk
// and store it under the snapshot's object key.
type SnapshotRequested struct {
	SnapshotUUID string `json:"snapshot_uuid"`
	VMUUID       string `json:"vm_uuid"`
	NodeName     string `json:"node_name"`
}
