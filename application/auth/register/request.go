package register

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Request struct {
	Identity string `json:"identity"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if !user.IsValidEmail(r.Identity) {
		validationErrors["identity"] = "invalid_email"
	}

	return validationErrors
}
