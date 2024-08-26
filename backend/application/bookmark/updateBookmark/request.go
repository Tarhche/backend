package updateBookmark

import "github.com/khanzadimahdi/testproject/domain/bookmark"

type validationErrors map[string]string

type Request struct {
	Keep       bool   `json:"keep"`
	Title      string `json:"title"`
	ObjectType string `json:"object_type"`
	ObjectUUID string `json:"object_uuid"`
	OwnerUUID  string `json:"-"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Title) == 0 {
		errors["title"] = "title is required"
	}

	if r.ObjectType != bookmark.ObjectTypeArticle {
		errors["object_type"] = "object type is not supported"
	}

	if len(r.ObjectUUID) == 0 {
		errors["object_uuid"] = "object uuid is required"
	}

	if len(r.OwnerUUID) == 0 {
		errors["owner_uuid"] = "owner uuid is required"
	}

	return len(errors) == 0, errors
}
