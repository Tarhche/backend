package restoreVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Request asks this node to replace one of its VMs' disk from a snapshot.
type Request struct {
	VMUUID       string  `json:"vm_uuid"`
	SnapshotUUID string  `json:"snapshot_uuid"`
	Spec         vm.Spec `json:"spec"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	if len(r.SnapshotUUID) == 0 {
		validationErrors["snapshot_uuid"] = "required_field"
	}

	if !r.Spec.Kind.IsValid() {
		validationErrors["spec.kind"] = "invalid_value"
	}

	if len(r.Spec.Image) == 0 {
		validationErrors["spec.image"] = "required_field"
	}

	return validationErrors
}
