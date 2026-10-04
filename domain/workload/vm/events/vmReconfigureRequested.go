package events

const VMReconfigureRequestedName = "workloadVMReconfigureRequested"

// VMReconfigureRequested asks a node to give a VM the ports, network and
// resources its spec now says, restarting it when the engine cannot apply them
// to a running VM.
type VMReconfigureRequested struct {
	VMUUID   string `json:"vm_uuid"`
	NodeName string `json:"node_name"`
	Spec     Spec   `json:"spec"`
}
