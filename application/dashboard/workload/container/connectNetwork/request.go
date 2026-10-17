package connectNetwork

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
)

// Request attaches a container to another network of its VM.
type Request struct {
	VMUUID string `json:"-"`

	// ID is the container's id or its name.
	ID string `json:"-"`

	// Network is the network's id or its name.
	Network string `json:"network"`

	// Aliases are the names its neighbours on that network reach it by, beside
	// its own.
	Aliases []string `json:"aliases,omitempty"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(strings.TrimSpace(r.Network)) == 0 {
		validationErrors["network"] = "required_field"
	}

	return validationErrors
}
