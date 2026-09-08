package getEndpoint

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

type Request struct {
	// Slug is the name the container is addressed by from outside.
	Slug string `json:"slug"`

	// Port is the container's own port that was asked for. Zero asks for the
	// lowest one it exposes, which is what a hostname naming no port means.
	Port port.Port `json:"port"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Slug) == 0 {
		validationErrors["slug"] = "required_field"
	}

	return validationErrors
}
