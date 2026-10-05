package runStackAction

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

// Request asks this node to run one compose command on a stack in one of its
// Docker VMs.
type Request struct {
	StackUUID string       `json:"stack_uuid"`
	VMUUID    string       `json:"vm_uuid"`
	Action    stack.Action `json:"action"`

	// Project is the compose project, which is the stack's slug, and Compose
	// the YAML as it was given.
	Project string `json:"project"`
	Compose string `json:"compose"`

	// RemoveVolumes takes the project's volumes away with it, on a down.
	RemoveVolumes bool `json:"remove_volumes"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.StackUUID) == 0 {
		validationErrors["stack_uuid"] = "required_field"
	}

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	if !r.Action.IsValid() {
		validationErrors["action"] = "invalid_value"
	}

	if len(r.Project) == 0 {
		validationErrors["project"] = "required_field"
	}

	if len(r.Compose) == 0 {
		validationErrors["compose"] = "required_field"
	}

	return validationErrors
}
