package getTaskLogs

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

// maxLimit caps one read, so a container with a long history is paged through
// rather than loaded whole.
const maxLimit uint = 1000

type Request struct {
	UUID string `json:"-"`

	// After pages forward through a container's history: the next read asks
	// for what was written after the last line it already has.
	After time.Time `json:"after"`
	Limit uint      `json:"limit"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if r.Limit > maxLimit {
		validationErrors["limit"] = "invalid_value"
	}

	return validationErrors
}
