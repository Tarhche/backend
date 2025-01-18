package updateComment

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
)

type Request struct {
	UUID       string    `json:"uuid"`
	Body       string    `json:"body"`
	AuthorUUID string    `json:"author_uuid"`
	ParentUUID string    `json:"parent_uuid"`
	ObjectUUID string    `json:"object_uuid"`
	ObjectType string    `json:"object_type"`
	ApprovedAt time.Time `json:"approved_at"`
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

	if r.ObjectType != comment.ObjectTypeArticle {
		validationErrors["object_type"] = "invalid_value"
	}

	if len(r.ObjectUUID) == 0 {
		validationErrors["object_uuid"] = "required_field"
	}

	return validationErrors
}
