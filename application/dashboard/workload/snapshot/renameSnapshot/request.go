package renameSnapshot

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request renames a snapshot, which is all there is to change about one.
type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`

	Name string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.Name(validationErrors, "name", r.Name)

	return validationErrors
}
