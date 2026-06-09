package createlanguage

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Code) == 0 {
		validationErrors["code"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	return validationErrors
}
