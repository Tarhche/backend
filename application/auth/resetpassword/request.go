package resetpassword

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Token) == 0 {
		validationErrors["token"] = "required_field"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	return validationErrors
}
