package deletetask

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	UUID string `json:"uuid"`

	// Force removes a container that is still running, rather than refusing
	// until it has stopped. The worker takes a running container down either
	// way, so this is about what the caller asked for: "delete this" rather
	// than "delete this once it is finished".
	Force bool `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
