package updateComment

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/comment"
)

type validationErrors map[string]string

type Request struct {
	UUID       string    `json:"uuid"`
	Body       string    `json:"body"`
	AuthorUUID string    `json:"author_uuid"`
	ParentUUID string    `json:"parent_uuid"`
	ObjectUUID string    `json:"object_uuid"`
	ObjectType string    `json:"object_type"`
	ApprovedAt time.Time `json:"approved_at"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.UUID) == 0 {
		errors["uuid"] = "uuid is required"
	}

	if len(r.Body) == 0 {
		errors["body"] = "body is required"
	}

	if r.ObjectType != comment.ObjectTypeArticle {
		errors["object_type"] = "object type is not supported"
	}

	if len(r.ObjectUUID) == 0 {
		errors["object_uuid"] = "object_uuid is required"
	}

	return len(errors) == 0, errors
}
