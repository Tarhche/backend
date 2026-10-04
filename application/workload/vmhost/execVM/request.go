package execVM

import (
	"path"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	ID string `json:"id"`

	// Exec is the command to run beside the VM's task, and whether it has a
	// terminal.
	Exec guest.Exec `json:"exec"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	switch {
	case len(r.ID) == 0:
		validationErrors["id"] = "required_field"
	case !vm.IsID(r.ID):
		validationErrors["id"] = "invalid_value"
	}

	if len(r.Exec.Args) == 0 || len(r.Exec.Args[0]) == 0 {
		validationErrors["args"] = "required_field"
	}

	if len(r.Exec.WorkingDir) > 0 && !path.IsAbs(r.Exec.WorkingDir) {
		validationErrors["working_dir"] = "invalid_value"
	}

	return validationErrors
}
