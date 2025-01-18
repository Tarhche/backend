package register

import (
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

type Request struct {
	Identity string `json:"identity"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if !emailRegex.MatchString(r.Identity) {
		validationErrors["identity"] = "invalid_email"
	}

	return validationErrors
}
