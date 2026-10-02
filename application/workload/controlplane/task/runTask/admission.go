package runTask

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/domain"
)

// admission is a request held to what the platform allows as well as to what
// the request says about itself.
//
// Which classes a task may be run with is configuration (WORKLOAD_RUNTIMES)
// rather than syntax, so the request cannot answer it alone. Asking it here,
// as part of the validation rather than after it, means a class that is not
// allowed is refused the way every other field is, translated, under the field
// the caller sent it in.
type admission struct {
	*Request

	classes allowed.Classes
}

var _ domain.Validatable = admission{}

func (a admission) Validate() domain.ValidationErrors {
	validationErrors := a.Request.Validate()

	if _, refused := validationErrors["runtime"]; refused {
		return validationErrors
	}

	// a task naming no class gets the default, which is always allowed, so
	// only a class somebody named can be turned away here.
	if !a.classes.Allows(a.classes.Resolve(a.Runtime)) {
		validationErrors["runtime"] = "invalid_value"
	}

	return validationErrors
}
