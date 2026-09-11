package getRunner

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	ID string `json:"id"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.ID) == 0 {
		validationErrors["id"] = "required_field"
	}

	return validationErrors
}
