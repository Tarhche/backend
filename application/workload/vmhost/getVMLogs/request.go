package getVMLogs

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	ID string `json:"id"`

	// After is the last line number the reader has: only what came after it
	// is read. Since, when it is not zero, is when the first line read may
	// have been written. With Follow, what comes is read as it comes.
	After  uint64    `json:"after"`
	Since  time.Time `json:"since"`
	Follow bool      `json:"follow"`
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

	return validationErrors
}
