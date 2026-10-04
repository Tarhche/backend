// Package events is what the control plane and the nodes tell each other about
// stacks, over JetStream.
package events

import "github.com/khanzadimahdi/testproject/domain/workload/stack"

const StackRequestedName = "workloadStackRequested"

// StackRequested asks the node holding a Docker VM to run a compose command on
// one of its stacks. It carries the YAML itself, so the node needs nothing but
// the message to do it.
type StackRequested struct {
	StackUUID string       `json:"stack_uuid"`
	VMUUID    string       `json:"vm_uuid"`
	NodeName  string       `json:"node_name"`
	Action    stack.Action `json:"action"`

	// Project is the compose project, which is the stack's slug.
	Project string `json:"project"`
	Compose string `json:"compose"`

	// RemoveVolumes takes the project's volumes away with it, on a down.
	RemoveVolumes bool `json:"remove_volumes,omitempty"`
}
