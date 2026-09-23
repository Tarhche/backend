package launchMachine

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

const (
	// maxTaps is how many networks one machine may be plugged into. A task
	// joins at most two; four leaves room without leaving it open.
	maxTaps = 4

	// maxDrives is how many disks one machine may have: its image and its
	// scratch disk, with room to spare.
	maxDrives = 4

	// minMemoryMiB is the least a machine boots in: the kernel and the agent
	// need some of it before the task gets any.
	minMemoryMiB = 64
)

// Request is a machine to launch. It comes from an orchestrator, which is
// trusted to ask for what its tasks need and nothing more, and is checked all
// the same: the launcher is the one holding privilege.
type Request struct {
	machine.Spec
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.ID) == 0:
		validationErrors["id"] = "required_field"
	case !machine.IsID(r.ID):
		validationErrors["id"] = "invalid_value"
	}

	switch {
	case len(r.Owner) == 0:
		validationErrors["owner"] = "required_field"
	case !machine.IsOwner(r.Owner):
		validationErrors["owner"] = "invalid_value"
	}

	if r.VCPUs < 1 {
		validationErrors["vcpus"] = "invalid_value"
	}

	if r.CPUQuota < 0 || r.CPUQuota > float64(r.VCPUs) {
		validationErrors["cpu_quota"] = "invalid_value"
	}

	if r.MemoryMiB < minMemoryMiB {
		validationErrors["memory_mib"] = "invalid_value"
	}

	if len(r.Taps) > maxTaps {
		validationErrors["taps"] = "too_many"
	}

	for _, tap := range r.Taps {
		if !machine.IsNetwork(tap.Network) {
			validationErrors["taps"] = "invalid_value"
		}
	}

	if len(r.Files.Kernel) == 0 {
		validationErrors["files.kernel"] = "required_field"
	}

	if len(r.Files.Initrd) == 0 {
		validationErrors["files.initrd"] = "required_field"
	}

	if len(r.Files.Drives) > maxDrives {
		validationErrors["files.drives"] = "too_many"
	}

	return validationErrors
}
