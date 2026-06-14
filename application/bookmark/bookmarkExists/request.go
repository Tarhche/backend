package bookmarkExists

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

type Request struct {
	ObjectType   string `json:"object_type"`
	ObjectUUID   string `json:"object_uuid"`
	LanguageCode string `json:"language_code"`
	OwnerUUID    string `json:"-"`
}

func (r *Request) Validate() domain.ValidationErrors {
	errors := make(domain.ValidationErrors)

	if r.ObjectType != bookmark.ObjectTypeArticle {
		errors["object_type"] = "invalid_value"
	}

	if len(r.ObjectUUID) == 0 {
		errors["object_uuid"] = "required_field"
	}

	if len(r.LanguageCode) == 0 {
		errors["language_code"] = "required_field"
	}

	if len(r.OwnerUUID) == 0 {
		errors["owner_uuid"] = "required_field"
	}

	return errors
}
