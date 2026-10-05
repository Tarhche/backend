package createVolume

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// Request is a volume to create inside a Docker VM. It lives on the VM's
// disk, so it is kept across a restart only if that disk is persistent.
type Request struct {
	VMUUID string `json:"-"`

	Name string `json:"name"`

	// Driver is local, which is also what empty is: a volume lives on the
	// VM's own disk.
	Driver string `json:"driver,omitempty"`

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

	if len(r.Driver) > 0 && r.Driver != "local" {
		validationErrors["driver"] = "invalid_volume_driver"
	}

	return validationErrors
}

// Spec is the volume as docker is asked for it.
func (r *Request) Spec() docker.VolumeSpec {
	return docker.VolumeSpec{
		Name:   r.Name,
		Driver: r.Driver,
		Labels: r.Labels,
	}
}
