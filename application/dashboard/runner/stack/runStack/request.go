package runStack

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request is a set of services to run together, in the shape a compose file
// has.
type Request struct {
	spec.Stack

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	return r.Stack.Validate()
}
