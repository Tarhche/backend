package takeSnapshot

import "github.com/khanzadimahdi/testproject/domain"

// Request asks this node to take a snapshot of one of its VMs' disk and store
// it under the snapshot's object key.
type Request struct {
	SnapshotUUID string `json:"snapshot_uuid"`
	VMUUID       string `json:"vm_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.SnapshotUUID) == 0 {
		validationErrors["snapshot_uuid"] = "required_field"
	}

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	return validationErrors
}
