package createuser

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
	Password string `json:"password"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Email) == 0 {
		validationErrors["email"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	return validationErrors
}
