package restoreVM

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`

	// SnapshotUUID is the snapshot whose disk replaces the VM's: one of its
	// owner's, of the same kind and engine, and no larger than its disk.
	SnapshotUUID string `json:"snapshot_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.SnapshotUUID) == 0 {
		validationErrors["snapshot_uuid"] = "required_field"
	}

	return validationErrors
}
