package attachResource

import "github.com/khanzadimahdi/testproject/domain"

// Request asks for a stream, a terminal say, in a resource.
type Request struct {
	// Kind and Action are the resource's kind and the stream action asked
	// for, as the route they were asked on names them.
	Kind   string `json:"-"`
	Action string `json:"-"`

	UUID string `json:"uuid"`

	// OwnerUUID is who is asking, taken from the token they presented.
	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}
