package events

const VMRestartRequestedName = "workloadVMRestartRequested"

// VMRestartRequested asks a node to stop a VM and boot it again in place.
type VMRestartRequested struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
}
