package updateConfig

import "github.com/khanzadimahdi/testproject/domain"

type validationErrors map[string]string

type Request struct {
	UserDefaultRoles []string `json:"user_default_roles"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UserDefaultRoles) == 0 {
		validationErrors["user_default_roles"] = "required_field"
	}

	return validationErrors
}
