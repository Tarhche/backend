package deleteStack

import "github.com/khanzadimahdi/testproject/domain"

// Request names a stack to take down, as one person's own when OwnerUUID is
// set.
type Request struct {
	OwnerUUID string
	UUID      string

	// RemoveVolumes takes the project's volumes away with it.
	RemoveVolumes bool
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
