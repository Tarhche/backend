package restoreVM

import "github.com/khanzadimahdi/testproject/domain"

// Request is a VM to restore from a snapshot, as one person's own when
// OwnerUUID is set.
type Request struct {
	OwnerUUID    string `json:"-"`
	UUID         string `json:"-"`
	SnapshotUUID string `json:"snapshot_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.SnapshotUUID) == 0 {
		validationErrors["snapshot_uuid"] = "required_field"
	}

	return validationErrors
}
