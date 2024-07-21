package createComment

import "github.com/khanzadimahdi/testproject/domain/comment"

type validationErrors map[string]string

type Request struct {
	Body       string `json:"body"`
	AuthorUUID string `json:"-"`
	ParentUUID string `json:"parent_uuid"`
	ObjectUUID string `json:"object_uuid"`
	ObjectType string `json:"object_type"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

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
