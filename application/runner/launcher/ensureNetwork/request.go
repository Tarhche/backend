package ensureNetwork

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Request struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`

	// Masquerade is whether the network routes out to the internet.
	Masquerade bool `json:"masquerade"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	return validateNetwork(r.Owner, r.Name)
}

// validateNetwork checks that a network is named the way only the runner's
// own are, so nothing asked of the launcher can reach a network that is not.
func validateNetwork(owner string, name string) domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(owner) == 0:
		validationErrors["owner"] = "required_field"
	case !machine.IsOwner(owner):
		validationErrors["owner"] = "invalid_value"
	}

	switch {
	case len(name) == 0:
		validationErrors["name"] = "required_field"
	case !machine.IsNetwork(name):
		validationErrors["name"] = "invalid_value"
	}

	return validationErrors
}
