package updateUserComment

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	UUID     string `json:"uuid"`
	Body     string `json:"body"`
	UserUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.Body) == 0 {
		validationErrors["body"] = "required_field"
	}

	if len(r.UserUUID) == 0 {
		validationErrors["user_uuid"] = "required_field"
	}

	return validationErrors
}
