package getArticlesByAuthor

import (
	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Request struct {
	AuthorUUID string
	Username   string
	Page       uint
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.AuthorUUID) == 0 && len(r.Username) == 0 {
		validationErrors["author"] = "required_field"
	}

	if len(r.AuthorUUID) > 0 {
		if _, err := uuid.FromString(r.AuthorUUID); err != nil {
			validationErrors["uuid"] = "invalid_value"
		}
	}

	if len(r.Username) > 0 && !user.IsValidUsername(r.Username) {
		validationErrors["username"] = "invalid_value"
	}

	return validationErrors
}
