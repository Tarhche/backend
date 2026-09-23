package removeNetwork

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Request struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.Owner) == 0:
		validationErrors["owner"] = "required_field"
	case !machine.IsOwner(r.Owner):
		validationErrors["owner"] = "invalid_value"
	}

	switch {
	case len(r.Name) == 0:
		validationErrors["name"] = "required_field"
	case !machine.IsNetwork(r.Name):
		validationErrors["name"] = "invalid_value"
	}

	return validationErrors
}
