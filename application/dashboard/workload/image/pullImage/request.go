package pullImage

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request is an image to pull into a Docker VM.
type Request struct {
	VMUUID string `json:"-"`

	// Reference is the image, as name[:tag] or name@digest.
	Reference string `json:"reference"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.Image(validationErrors, "reference", r.Reference)

	return validationErrors
}
