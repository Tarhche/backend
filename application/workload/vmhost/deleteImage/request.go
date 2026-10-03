package deleteImage

import (
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
)

// digestPattern is what an image's digest looks like: an algorithm and its
// hex, as OCI writes them.
var digestPattern = regexp.MustCompile(`^[a-z0-9]+(?:[+._-][a-z0-9]+)*:[a-zA-Z0-9=_-]{1,256}$`)

type Request struct {
	Digest string `json:"digest"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.Digest) == 0:
		validationErrors["digest"] = "required_field"
	case !digestPattern.MatchString(r.Digest):
		validationErrors["digest"] = "invalid_value"
	}

	return validationErrors
}
