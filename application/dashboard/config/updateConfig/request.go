package updateConfig

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	UserDefaultRoles    []string `json:"user_default_roles"`
	DefaultLanguageCode string   `json:"default_language_code"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UserDefaultRoles) == 0 {
		validationErrors["user_default_roles"] = "required_field"
	}

	if len(r.DefaultLanguageCode) == 0 {
		validationErrors["default_language_code"] = "required_field"
	}

	return validationErrors
}
