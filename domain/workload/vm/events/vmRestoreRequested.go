package events

const VMRestoreRequestedName = "workloadVMRestoreRequested"

// VMRestoreRequested asks a node to replace a VM's disk from a snapshot. The
// VM keeps its uuid, its slug and its ports.
type VMRestoreRequested struct {
	VMUUID       string `json:"vm_uuid"`
	NodeName     string `json:"node_name"`
	SnapshotUUID string `json:"snapshot_uuid"`
	Spec         Spec   `json:"spec"`
}
