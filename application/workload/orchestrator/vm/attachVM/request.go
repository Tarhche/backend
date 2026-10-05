package attachVM

import "github.com/khanzadimahdi/testproject/domain"

// Request asks for a terminal in a VM.
type Request struct {
	UUID string `json:"uuid"`

	// OwnerUUID is who is asking, taken from the token they presented.
	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
