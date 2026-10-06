package createContainer

import (
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Request is a container to create for OwnerUUID, in the Docker VM VM chooses.
type Request struct {
	OwnerUUID string `json:"-"`

	VM        dockervm.Choice           `json:"vm"`
	Container noderequest.ContainerSpec `json:"container"`
}

var _ domain.Validatable = &Request{}

// Validate checks what the control plane can tell from the request alone.
// Everything else about the container is dockerd's to accept or refuse.
func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	if len(strings.TrimSpace(r.Container.Image)) == 0 {
		validationErrors["container.image"] = "required_field"
	}

	if len(r.VM.UUID) > 0 && r.VM.New != nil {
		validationErrors["vm"] = "vm_or_new_vm"
	}

	return validationErrors
}
