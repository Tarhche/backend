package updateBookmark

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type Request struct {
	Keep       bool   `json:"keep"`
	Title      string `json:"title"`
	ObjectType string `json:"object_type"`
	ObjectUUID string `json:"object_uuid"`
	OwnerUUID  string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Title) == 0 {
		validationErrors["title"] = "required_field"
	}

	if r.ObjectType != bookmark.ObjectTypeArticle {
		validationErrors["object_type"] = "invalid_value"
	}

	if len(r.ObjectUUID) == 0 {
		validationErrors["object_uuid"] = "required_field"
	}

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	return validationErrors
}
