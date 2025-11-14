package verify

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Repassword string `json:"repassword"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Token) == 0 {
		validationErrors["token"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.Username) == 0 {
		validationErrors["username"] = "required_field"
	}

	if len(r.Password) == 0 {
		validationErrors["password"] = "required_field"
	}

	if len(r.Repassword) == 0 || len(r.Password) != len(r.Repassword) || r.Password != r.Repassword {
		validationErrors["repassword"] = "repassword"
	}

	return validationErrors
}
