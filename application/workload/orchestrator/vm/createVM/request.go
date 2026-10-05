package createVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Request asks this node to create a VM and boot it.
type Request struct {
	VMUUID string  `json:"vm_uuid"`
	Spec   vm.Spec `json:"spec"`

	// SnapshotUUID, when set, creates the VM from that snapshot's archive
	// rather than from its image: a snapshot restored as a new VM.
	SnapshotUUID string `json:"snapshot_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	if !r.Spec.Kind.IsValid() {
		validationErrors["spec.kind"] = "invalid_value"
	}

	if len(r.Spec.Image) == 0 {
		validationErrors["spec.image"] = "required_field"
	}

	return validationErrors
}
