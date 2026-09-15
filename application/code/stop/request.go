package stop

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	ID       string `json:"id"`
	TaskUUID string `json:"task_uuid"`
}

// Eunsure Request implements the Validatable interface.
var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.TaskUUID) == 0 {
		validationErrors["task_uuid"] = "required_field"
	}

	return validationErrors
}
