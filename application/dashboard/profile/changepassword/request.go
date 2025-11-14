package changepassword

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	UserUUID        string `json:"-"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UserUUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.NewPassword) == 0 {
		validationErrors["current_password"] = "required_field"
	}

	if len(r.NewPassword) == 0 {
		validationErrors["new_password"] = "required_field"
	}

	return validationErrors
}
