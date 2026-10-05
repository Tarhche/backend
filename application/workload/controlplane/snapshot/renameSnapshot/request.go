package renameSnapshot

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

// maxNameLength keeps a name to something a listing can show.
const maxNameLength = 100

// Request is a snapshot's new name, as one person's own when OwnerUUID is set.
type Request struct {
	OwnerUUID string `json:"-"`
	UUID      string `json:"-"`
	Name      string `json:"name"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	switch name := strings.TrimSpace(r.Name); {
	case len(name) == 0:
		validationErrors["name"] = "required_field"
	case len(name) > maxNameLength:
		validationErrors["name"] = "invalid_name"
	}

	return validationErrors
}
