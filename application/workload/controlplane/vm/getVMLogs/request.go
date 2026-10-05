package getVMLogs

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

// Request is the tail of a VM's log, as one person's own when OwnerUUID is
// set.
type Request struct {
	OwnerUUID string
	UUID      string

	// Since leaves out the lines written before it; zero is from the start.
	Since time.Time

	// Tail keeps the last lines; zero, or more than a reply can carry, is as
	// many as a reply can carry.
	Tail uint
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
