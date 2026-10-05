package createSnapshot

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

// maxNameLength keeps a name to something a listing can show.
const maxNameLength = 100

// Request is a snapshot to take of a VM, for OwnerUUID, of one of their own
// VMs. An empty OwnerUUID takes it of anybody's, for the VM's owner.
type Request struct {
	OwnerUUID string `json:"-"`
	VMUUID    string `json:"-"`
	Name      string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	switch name := strings.TrimSpace(r.Name); {
	case len(name) == 0:
		validationErrors["name"] = "required_field"
	case len(name) > maxNameLength:
		validationErrors["name"] = "invalid_name"
	}

	return validationErrors
}
