package login

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Identity string `json:"identity"`
	Password string `json:"password"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Identity) == 0 {
		validationErrors["identity"] = "required_field"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	return validationErrors
}
