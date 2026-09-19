package providerredirect

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	// Provider names somebody else's login page. It is not asked for in a body:
	// it is the one in the address the caller reached.
	Provider string `json:"provider"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Provider) == 0 {
		validationErrors["provider"] = "required_field"
	}

	return validationErrors
}
