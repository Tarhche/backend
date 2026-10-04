package startVM

import "github.com/khanzadimahdi/testproject/domain"

// Request asks this node to boot a VM it holds.
type Request struct {
	VMUUID string `json:"vm_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	return validationErrors
}
