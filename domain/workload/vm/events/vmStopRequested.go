package events

const VMStopRequestedName = "workloadVMStopRequested"

// VMStopRequested asks a node to stop a VM, keeping its disk.
type VMStopRequested struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
}
