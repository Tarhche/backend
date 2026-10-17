package createVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/input"
)

// Request is a VM to create. It is always created for whoever asks.
type Request struct {
	Name string `json:"name"`

	// Kind is machine, an OS image somebody opens a terminal in, or docker,
	// the image containers and stacks are run in.
	Kind string `json:"kind"`

	// Image is the OCI reference a machine boots from; empty takes its
	// kind's default, and a Docker VM is always the workload's own image.
	Image string `json:"image,omitempty"`

	Resources input.Resources `json:"resources"`

	// Ports are the guest ports the ingress serves, while ingress allows it.
	Ports   []uint        `json:"ports"`
	Network input.Network `json:"network"`

	// PersistentDisk keeps what is written to the disk across a stop and a
	// start; without it the disk is as the image left it on every start.
	PersistentDisk bool `json:"persistent_disk"`

	// LifetimeSeconds is how long the VM is kept before it is deleted, and
	// zero keeps it until somebody deletes it.
	LifetimeSeconds int64 `json:"lifetime_seconds"`

	// SnapshotUUID makes the VM from one of the caller's snapshots: its kind
	// and image are the snapshot's, and its disk the larger of the one asked
	// for and the snapshot's.
	SnapshotUUID string `json:"snapshot_uuid,omitempty"`

	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	input.Name(validationErrors, "name", r.Name)

	switch {
	case len(r.Kind) == 0:
		validationErrors["kind"] = "required_field"
	case !vm.Kind(r.Kind).IsValid():
		validationErrors["kind"] = "invalid_kind"
	}

	if len(r.Image) > 0 {
		input.Image(validationErrors, "image", r.Image)
	}

	// a VM made from a snapshot takes the snapshot's disk when it asks for
	// none, since that is the least it can be restored onto.
	r.Resources.Validate(validationErrors, "resources", len(r.SnapshotUUID) > 0)

	input.Ports(validationErrors, "ports", r.Ports)
	r.Network.Validate(validationErrors, "network")
	input.Lifetime(validationErrors, "lifetime_seconds", r.LifetimeSeconds)

	return validationErrors
}
