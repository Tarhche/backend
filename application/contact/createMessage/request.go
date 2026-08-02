package createMessage

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/validator/rules"
)

type Request struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(strings.TrimSpace(r.Subject)) == 0 {
		validationErrors["subject"] = "required_field"
	}

	if len(strings.TrimSpace(r.Body)) == 0 {
		validationErrors["body"] = "required_field"
	}

	// The sender is anonymous, so at least one way of reaching them back is
	// required; whichever one they filled in still has to be well-formed.
	if len(r.Email) == 0 && len(r.Phone) == 0 {
		validationErrors["email"] = "email_or_phone_required"
		validationErrors["phone"] = "email_or_phone_required"

		return validationErrors
	}

	if len(r.Email) > 0 && !rules.IsValidEmail(r.Email) {
		validationErrors["email"] = "invalid_email"
	}

	if len(r.Phone) > 0 && !rules.IsValidPhoneNumber(r.Phone) {
		validationErrors["phone"] = "invalid_phone_number"
	}

	return validationErrors
}
