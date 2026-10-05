package stopVM

import "github.com/khanzadimahdi/testproject/domain"

// Request names a VM, as one person's own when OwnerUUID is set.
type Request struct {
	OwnerUUID string
	UUID      string
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
