package updateuser

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	UserUUID string `json:"uuid"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UserUUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.Email) == 0 {
		validationErrors["email"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	return validationErrors
}
