package createVM

import (
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// MaxPorts is the most ports a VM may expose through the ingress.
	MaxPorts = 16

	// maxNameLength keeps a name to something a listing can show.
	maxNameLength = 100

	// maxPort is the highest port there is.
	maxPort = 65535
)

// Request is a VM to create, for OwnerUUID.
type Request struct {
	OwnerUUID string `json:"-"`

	Name string  `json:"name"`
	Kind vm.Kind `json:"kind"`

	// Image is what a machine VM boots from; empty is the default image. A
	// Docker VM always boots from the Docker image.
	Image string `json:"image"`

	Resources      Resources   `json:"resources"`
	Ports          []port.Port `json:"ports"`
	Network        Network     `json:"network"`
	PersistentDisk bool        `json:"persistent_disk"`

	// LifetimeSeconds is how long the VM is kept; zero keeps it until it is
	// deleted.
	LifetimeSeconds int64 `json:"lifetime_seconds"`

	// SnapshotUUID makes the VM from a snapshot of the same owner: its kind and
	// image are the snapshot's, and its disk is the larger of the one asked for
	// and the snapshot's.
	SnapshotUUID string `json:"snapshot_uuid"`
}

// Resources are whole vCPUs, and bytes.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

func (r Resources) VM() vm.Resources {
	return vm.Resources{CPUs: r.CPUs, Memory: r.Memory, Disk: r.Disk}
}

// Network is allow or deny each way. Either left empty is allowed, which is
// what a VM somebody made to reach and be reached is usually for.
type Network struct {
	Ingress vm.Access `json:"ingress"`
	Egress  vm.Access `json:"egress"`
}

func (n Network) VM() vm.Network {
	network := vm.Network{Ingress: n.Ingress, Egress: n.Egress}

	if len(network.Ingress) == 0 {
		network.Ingress = vm.AccessAllow
	}

	if len(network.Egress) == 0 {
		network.Egress = vm.AccessAllow
	}

	return network
}

var _ domain.Validatable = &Request{}

// Validate checks what can be told from the request alone. What it asks for
// against what a VM and a person may be given, and against the snapshot it
// names, is checked by the use case, which knows both.
func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	switch name := strings.TrimSpace(r.Name); {
	case len(name) == 0:
		validationErrors["name"] = "required_field"
	case len(name) > maxNameLength:
		validationErrors["name"] = "invalid_name"
	}

	// a VM made from a snapshot is of the snapshot's kind, so it need not say.
	switch {
	case len(r.Kind) == 0 && len(r.SnapshotUUID) == 0:
		validationErrors["kind"] = "required_field"
	case len(r.Kind) > 0 && !r.Kind.IsValid():
		validationErrors["kind"] = "invalid_kind"
	}

	if code, ok := ValidatePorts(r.Ports); !ok {
		validationErrors["ports"] = code
	}

	if len(r.Network.Ingress) > 0 && !r.Network.Ingress.IsValid() {
		validationErrors["network.ingress"] = "invalid_access"
	}

	if len(r.Network.Egress) > 0 && !r.Network.Egress.IsValid() {
		validationErrors["network.egress"] = "invalid_access"
	}

	if r.LifetimeSeconds < 0 {
		validationErrors["lifetime_seconds"] = "invalid_lifetime"
	}

	return validationErrors
}

// ValidatePorts checks the ports a VM exposes: a port there is, and no more
// of them than a VM may have. The same port twice is the same port, and is not
// refused for it.
func ValidatePorts(ports []port.Port) (string, bool) {
	if len(ports) > MaxPorts {
		return "too_many_ports", false
	}

	for _, p := range ports {
		if p == 0 || p > maxPort {
			return "invalid_port", false
		}
	}

	return "", true
}
