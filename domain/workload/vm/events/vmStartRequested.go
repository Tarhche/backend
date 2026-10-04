package events

const VMStartRequestedName = "workloadVMStartRequested"

// VMStartRequested asks a node to boot a stopped VM.
type VMStartRequested struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
}
