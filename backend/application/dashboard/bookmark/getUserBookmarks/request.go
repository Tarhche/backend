package getUserBookmarks

import "github.com/khanzadimahdi/testproject/domain"

type Request struct {
	OwnerUUID string `json:"-"`
	Page      uint
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	return validationErrors
}
