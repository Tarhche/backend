package getNode

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Name string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	return validationErrors
}
