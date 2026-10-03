package stopVM

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	ID string `json:"id"`

	// Timeout is how long the task is given to end on its own.
	Timeout time.Duration `json:"timeout"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.ID) == 0:
		validationErrors["id"] = "required_field"
	case !vm.IsID(r.ID):
		validationErrors["id"] = "invalid_value"
	}

	if r.Timeout < 0 {
		validationErrors["timeout"] = "invalid_value"
	}

	return validationErrors
}
