package createuser

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Request struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	LanguageCode string `json:"language_code"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Email) == 0 {
		validationErrors["email"] = "required_field"
	} else if !user.IsValidEmail(r.Email) {
		validationErrors["email"] = "invalid_email"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.Username) == 0 {
		validationErrors["username"] = "required_field"
	} else if !user.IsValidUsername(r.Username) {
		validationErrors["username"] = "invalid_value"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	if len(r.LanguageCode) == 0 {
		validationErrors["language_code"] = "required_field"
	}

	return validationErrors
}
