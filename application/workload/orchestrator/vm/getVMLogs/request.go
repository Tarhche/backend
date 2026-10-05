package getVMLogs

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

// Request narrows what is read of a VM's log.
type Request struct {
	VMUUID string `json:"vm_uuid"`

	// Since leaves out the lines written before it; zero is from the start.
	Since time.Time `json:"since"`

	// Tail keeps only the last lines; zero is as many as a reply carries.
	Tail uint `json:"tail"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	return validationErrors
}
