package events

const VMDeleteRequestedName = "workloadVMDeleteRequested"

// VMDeleteRequested asks a node to remove a VM, disk and all. Its snapshots
// are not the node's to remove: they outlive it.
type VMDeleteRequested struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
}
