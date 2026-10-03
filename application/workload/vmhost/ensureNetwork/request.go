package ensureNetwork

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	Name       string `json:"name"`
	Masquerade bool   `json:"masquerade"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.Name) == 0:
		validationErrors["name"] = "required_field"
	case !vm.IsNetworkName(r.Name):
		validationErrors["name"] = "invalid_value"
	}

	return validationErrors
}
