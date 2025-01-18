package createComment

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
)

type Request struct {
	Body       string `json:"body"`
	AuthorUUID string `json:"-"`
	ParentUUID string `json:"parent_uuid"`
	ObjectUUID string `json:"object_uuid"`
	ObjectType string `json:"object_type"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Body) == 0 {
		validationErrors["body"] = "required_field"
	}

	if r.ObjectType != comment.ObjectTypeArticle {
		validationErrors["object_type"] = "invalid_value"
	}

	if len(r.ObjectUUID) == 0 {
		validationErrors["object_uuid"] = "required_field"
	}

	return validationErrors
}
