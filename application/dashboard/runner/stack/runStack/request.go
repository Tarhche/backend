package runStack

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
)

type Request struct {
	OwnerUUID string `json:"-"`

	spec.Stack
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	return r.Stack.Validate()
}
