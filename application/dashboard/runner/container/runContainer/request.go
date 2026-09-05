package runContainer

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request is one container to run, in the shape a compose service has, with the
// name it is to be known by.
type Request struct {
	Name string `json:"name"`

	spec.Service

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := r.Service.Validate("")

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	return validationErrors
}
