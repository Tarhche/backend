package endExec

import (
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// execPattern is what an agent may call a command: it is written into the
// path vmhost asks the agent on.
var execPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type Request struct {
	ID   string `json:"id"`
	Exec string `json:"exec"`

	// End is how long the command is given to end on its own, and then once
	// it has been asked to.
	End guest.EndExec `json:"end"`
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

	switch {
	case len(r.Exec) == 0:
		validationErrors["exec"] = "required_field"
	case !execPattern.MatchString(r.Exec):
		validationErrors["exec"] = "invalid_value"
	}

	if r.End.Grace < 0 || r.End.KillGrace < 0 {
		validationErrors["end"] = "invalid_value"
	}

	return validationErrors
}
