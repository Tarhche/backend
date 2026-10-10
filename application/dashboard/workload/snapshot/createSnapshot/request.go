package createSnapshot

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// Request is a snapshot to take of one of the caller's own VMs, which has to
// be running or stopped.
type Request struct {
	VMUUID string `json:"-"`
	Name   string `json:"name"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.Name(validationErrors, "name", r.Name)

	return validationErrors
}
