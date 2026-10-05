package updateVM

import (
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// maxNameLength keeps a name to something a listing can show.
const maxNameLength = 100

// Request is what changes about a VM. Anything left out stays as it is; its
// kind and its image never change, and its disk only grows.
type Request struct {
	OwnerUUID string `json:"-"`
	UUID      string `json:"-"`

	Name *string `json:"name,omitempty"`

	// LifetimeSeconds counts again from now; zero keeps the VM until it is
	// deleted.
	LifetimeSeconds *int64 `json:"lifetime_seconds,omitempty"`

	Ports *[]port.Port `json:"ports,omitempty"`

	// Network changes either way that it names; one it leaves empty stays.
	Network *createVM.Network `json:"network,omitempty"`

	Resources *createVM.Resources `json:"resources,omitempty"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if r.Name != nil {
		switch name := strings.TrimSpace(*r.Name); {
		case len(name) == 0:
			validationErrors["name"] = "required_field"
		case len(name) > maxNameLength:
			validationErrors["name"] = "invalid_name"
		}
	}

	if r.LifetimeSeconds != nil && *r.LifetimeSeconds < 0 {
		validationErrors["lifetime_seconds"] = "invalid_lifetime"
	}

	if r.Ports != nil {
		if code, ok := createVM.ValidatePorts(*r.Ports); !ok {
			validationErrors["ports"] = code
		}
	}

	if r.Network != nil {
		if len(r.Network.Ingress) > 0 && !r.Network.Ingress.IsValid() {
			validationErrors["network.ingress"] = "invalid_access"
		}

		if len(r.Network.Egress) > 0 && !r.Network.Egress.IsValid() {
			validationErrors["network.egress"] = "invalid_access"
		}
	}

	return validationErrors
}
