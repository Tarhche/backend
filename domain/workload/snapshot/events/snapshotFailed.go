package events

import "time"

const SnapshotFailedName = "workloadSnapshotFailed"

// SnapshotFailed says a snapshot could not be taken or stored. Nothing of it
// is left behind.
type SnapshotFailed struct {
	SnapshotUUID string    `json:"snapshot_uuid"`
	NodeName     string    `json:"node_name"`
	Reason       string    `json:"reason"`
	At           time.Time `json:"at"`
}
