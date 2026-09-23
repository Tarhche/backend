package terminateMachine

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Request struct {
	ID string `json:"id"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.ID) == 0:
		validationErrors["id"] = "required_field"
	case !machine.IsID(r.ID):
		validationErrors["id"] = "invalid_value"
	}

	return validationErrors
}
