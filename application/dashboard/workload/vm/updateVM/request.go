package updateVM

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/domain"
)

// Request is what changes about a VM. Anything left out stays as it is. Its
// kind and its image never change, and its disk only grows; a change to its
// ports, its network or its resources restarts a VM that is not stopped.
type Request struct {
	UUID      string `json:"-"`
	OwnerUUID string `json:"-"`

	Name *string `json:"name,omitempty"`

	// LifetimeSeconds counts again from now, and zero keeps the VM until it
	// is deleted.
	LifetimeSeconds *int64 `json:"lifetime_seconds,omitempty"`

	// Ports replace the ones it has; an empty list exposes none.
	Ports *[]uint `json:"ports,omitempty"`

	Network   *input.Network   `json:"network,omitempty"`
	Resources *input.Resources `json:"resources,omitempty"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if r.Name != nil {
		input.Name(validationErrors, "name", *r.Name)
	}

	if r.LifetimeSeconds != nil {
		input.Lifetime(validationErrors, "lifetime_seconds", *r.LifetimeSeconds)
	}

	if r.Ports != nil {
		input.Ports(validationErrors, "ports", *r.Ports)
	}

	if r.Network != nil {
		r.Network.Validate(validationErrors, "network")
	}

	if r.Resources != nil {
		r.Resources.Validate(validationErrors, "resources", false)
	}

	return validationErrors
}
