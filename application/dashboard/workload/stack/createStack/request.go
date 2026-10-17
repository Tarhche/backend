package createStack

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// MaxCompose is the longest file a stack is deployed from, in bytes: as long
// as the control plane reads a stack's file, which it refuses as too_large
// beyond that too.
const MaxCompose = stackKind.MaxFile

// Request is a stack to deploy into one of the caller's Docker VMs, from its
// file. It is always deployed for whoever asks.
//
// Which VM it goes in is the request's to say, as a container's is: one named
// by VMUUID, a new one described by VM, or neither, which makes a new one
// with the defaults.
type Request struct {
	Name string `json:"name"`

	// Compose is its file, YAML in the shape of a compose file, of which the
	// platform takes what it carries out itself: services, volumes and
	// networks. Anything else is refused under its path in the file
	// (compose.services.web.build). It names no project: what the stack's
	// graph makes is named under the stack's slug.
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
