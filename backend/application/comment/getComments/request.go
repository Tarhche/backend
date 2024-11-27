package getComments

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
)

type Request struct {
	Page       uint
	ObjectUUID string
	ObjectType string
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if r.ObjectType != comment.ObjectTypeArticle {
		validationErrors["object_type"] = "invalid_value"
	}

	if len(r.ObjectUUID) == 0 {
		validationErrors["object_uuid"] = "required_field"
	}

	return validationErrors
}
