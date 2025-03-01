package getTask

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// Request represents a request to get a task
type Request struct {
	UUID string `json:"uuid"`
}

var _ domain.Validatable = &Request{}

// Validate validates the request
func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
