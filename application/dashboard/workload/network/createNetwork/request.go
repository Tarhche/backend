package createNetwork

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// Request is a docker network to create inside a Docker VM. It reaches no
// further than the VM, whatever it is told.
type Request struct {
	VMUUID string `json:"-"`

	Name string `json:"name"`

	// Driver is bridge, which is also what empty is: a VM is a single docker
	// host, and no other driver has anything to join there.
	Driver string `json:"driver,omitempty"`

	// Internal keeps the containers on it from reaching anything outside it.
	Internal bool `json:"internal,omitempty"`

	Labels map[string]string `json:"labels,omitempty"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	} else {
		input.DockerName(validationErrors, "name", r.Name)
	}

	if len(r.Driver) > 0 && r.Driver != "bridge" {
		validationErrors["driver"] = "invalid_network_driver"
	}

	return validationErrors
}

// Spec is the network as docker is asked for it.
func (r *Request) Spec() docker.NetworkSpec {
	return docker.NetworkSpec{
		Name:     r.Name,
		Driver:   r.Driver,
		Internal: r.Internal,
		Labels:   r.Labels,
	}
}
