package deleteUserBookmark

import "github.com/khanzadimahdi/testproject/domain/bookmark"

type validationErrors map[string]string

type Request struct {
	ObjectType string `json:"object_type"`
	ObjectUUID string `json:"object_uuid"`
	OwnerUUID  string `json:"-"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

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
