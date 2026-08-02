package createnote

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	Body            string    `json:"body"`
	PublishedAt     time.Time `json:"published_at"`
	AuthorUUID      string    `json:"-"`
	Tags            []string  `json:"tags"`
	LanguageCode    string    `json:"language_code"`
	CorrelationUUID string    `json:"correlation_uuid"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Body) == 0 {
		validationErrors["body"] = "required_field"
	}

	if len(r.AuthorUUID) == 0 {
		validationErrors["author_uuid"] = "required_field"
	}

	if len(r.LanguageCode) == 0 {
		validationErrors["language_code"] = "required_field"
	}

	return validationErrors
}
