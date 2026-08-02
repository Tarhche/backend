package updatenote

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	CorrelationUUID string    `json:"correlation_uuid"`
	Body            string    `json:"body"`
	PublishedAt     time.Time `json:"published_at"`
	AuthorUUID      string    `json:"-"`
	Tags            []string  `json:"tags"`
	LanguageCode    string    `json:"language_code"`
	// OwnerUUID scopes the update to a single author's own notes. It is empty on
	// the routes guarded by the global notes permissions.
	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.CorrelationUUID) == 0 {
		validationErrors["correlation_uuid"] = "required_field"
	}

	if len(r.Body) == 0 {
		validationErrors["body"] = "required_field"
	}

	if len(r.AuthorUUID) == 0 {
		validationErrors["author"] = "required_field"
	}

	if len(r.LanguageCode) == 0 {
		validationErrors["language_code"] = "required_field"
	}

	return validationErrors
}
