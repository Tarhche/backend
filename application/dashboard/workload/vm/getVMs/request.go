package getVMs

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Page uint `json:"page"`

	// OwnerUUID narrows the listing to one person's own, and is empty for
	// everybody's.
	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	return validationErrors
}
