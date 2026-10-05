package getStack

import "github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"

// Response is the stack and the containers compose made for it, read from its
// VM as they are now.
type Response struct {
	presenter.Stack

	Containers []presenter.Container `json:"containers"`

	// VMNotRunning says there are no containers to show because the stack's
	// VM is not running, rather than because the stack has none.
	VMNotRunning bool `json:"vm_not_running"`
}
