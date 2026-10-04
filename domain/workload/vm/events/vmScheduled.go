package events

const VMScheduledName = "workloadVMScheduled"

// VMScheduled asks a node to create a VM and boot it.
type VMScheduled struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
	Spec     Spec   `json:"spec"`

	// SnapshotUUID, when set, creates the VM from that snapshot's archive
	// rather than from its image.
	SnapshotUUID string `json:"snapshot_uuid,omitempty"`
}
