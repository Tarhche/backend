package createStack

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// MaxCompose is the longest compose file a stack is deployed from, in bytes.
const MaxCompose = 256 << 10

// Request is a compose project to deploy into one of the caller's Docker VMs.
// It is always deployed for whoever asks.
//
// Which VM it goes in is the request's to say, as a container's is: one named
// by VMUUID, a new one described by VM, or neither.
type Request struct {
	Name string `json:"name"`

	// Compose is the YAML, as it would be handed to docker compose. What it
	// calls its project is ignored: the stack's slug is the project.
	Compose string `json:"compose"`

	VMUUID string             `json:"vm_uuid,omitempty"`
	VM     *input.NewDockerVM `json:"vm,omitempty"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.Name(validationErrors, "name", r.Name)

	switch {
	case len(strings.TrimSpace(r.Compose)) == 0:
		validationErrors["compose"] = "required_field"
	case len(r.Compose) > MaxCompose:
		validationErrors["compose"] = "too_large"
	}

	input.DockerVM(validationErrors, r.VMUUID, r.VM)

	return validationErrors
}
