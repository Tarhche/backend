package getArticlesByHashtag

import (
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Hashtag string
	Page    uint
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Hashtag) == 0 {
		validationErrors["hashtag"] = "required_field"
	}

	return validationErrors
}
