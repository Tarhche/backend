package refresh

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Token string `json:"token"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Token) == 0 {
		validationErrors["token"] = "required_field"
	}

	return validationErrors
}
