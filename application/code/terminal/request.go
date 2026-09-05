package terminal

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// Request opens a terminal in the container a snippet is running in.
//
// It names that container by uuid, which the run it came from reported and
// nobody else was told: a snippet's container is answered for by whoever ran
// it, and by nothing else.
type Request struct {
	ID            string   `json:"id"`
	ContainerUUID string   `json:"container_uuid"`
	Command       []string `json:"command"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.ContainerUUID) == 0 {
		validationErrors["container_uuid"] = "required_field"
	}

	return validationErrors
}
