package events

import "time"

const SnapshotCompletedName = "workloadSnapshotCompleted"

// SnapshotCompleted says a snapshot is stored and can be restored.
type SnapshotCompleted struct {
	SnapshotUUID string `json:"snapshot_uuid"`
	NodeName     string `json:"node_name"`

	// Size is how many bytes were stored, and Engine what wrote the archive,
	// which only the same engine can restore.
	Size   int64  `json:"size"`
	Engine string `json:"engine"`

	// Disk is the disk, in bytes, a restore needs at least.
	Disk uint64 `json:"disk"`

	At time.Time `json:"at"`
}
