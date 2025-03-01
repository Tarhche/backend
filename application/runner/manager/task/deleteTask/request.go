package deletetask

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	UUID string `json:"uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
