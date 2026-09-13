package runTask

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Name      string `json:"name"`
	OwnerUUID string `json:"-"`

	spec.Service
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := r.Service.Validate("")

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	return validationErrors
}
