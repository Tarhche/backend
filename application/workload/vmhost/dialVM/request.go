package dialVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	ID   string `json:"id"`
	Port uint16 `json:"port"`
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

	if r.Port == 0 {
		validationErrors["port"] = "required_field"
	}

	return validationErrors
}
