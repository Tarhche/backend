package prepareImage

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

// maxReference bounds an image reference, which is registry, repository, tag
// and digest together.
const maxReference = 512

type Request struct {
	Image string `json:"image"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.Image) == 0:
		validationErrors["image"] = "required_field"
	case len(r.Image) > maxReference || strings.ContainsAny(r.Image, " \t\r\n"):
		validationErrors["image"] = "invalid_value"
	}

	return validationErrors
}
