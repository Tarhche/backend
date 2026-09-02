package runStack

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request is a stack to run, in the shape a compose file has.
type Request struct {
	spec.Stack

	OwnerUUID string `json:"owner_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := r.Stack.Validate()

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	return validationErrors
}
