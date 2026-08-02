package register

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/validator/rules"
)

type Request struct {
	Identity string `json:"identity"`

	// LanguageCode is the language the registration email should be sent in. It
	// is resolved from the request (not the JSON body) by the handler.
	LanguageCode string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if !rules.IsValidEmail(r.Identity) {
		validationErrors["identity"] = "invalid_email"
	}

	return validationErrors
}
