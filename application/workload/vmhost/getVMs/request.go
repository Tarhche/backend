package getVMs

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

// maxLabels bounds the label filters one listing may carry.
const maxLabels = 16

type Request struct {
	// Labels are filters, each key=value; a VM has to carry every one.
	Labels []string `json:"labels"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Labels) > maxLabels {
		validationErrors["label"] = "exceeds_limit"

		return validationErrors
	}

	for _, filter := range r.Labels {
		if key, _, found := strings.Cut(filter, "="); !found || len(key) == 0 {
			validationErrors["label"] = "invalid_value"
		}
	}

	return validationErrors
}
