package getContentsByHashtag

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Hashtag      string
	LanguageCode string
	Page         uint
	// Type selects which kind of content to list: TypeArticle or TypeNote. Empty
	// lets the use case pick — see Response.Type.
	Type string
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Hashtag) == 0 {
		validationErrors["hashtag"] = "required_field"
	}

	if len(r.Type) > 0 && r.Type != TypeArticle && r.Type != TypeNote {
		validationErrors["type"] = "invalid_value"
	}

	return validationErrors
}
