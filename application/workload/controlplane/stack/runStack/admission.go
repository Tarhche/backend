package runStack

import (
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/domain"
)

// admission is a stack held to what the platform allows as well as to what the
// stack says about itself.
//
// The specification has already refused two services naming two classes. What
// it could not tell is whether a service naming a class and one naming none
// agree, which depends on the platform's default, and whether the one class
// they come to is a class a task may be run with at all. Both are asked here,
// once the default is known, as part of the validation, so they are refused
// the way every other field is.
type admission struct {
	*Request

	classes allowed.Classes
}

var _ domain.Validatable = admission{}

func (a admission) Validate() domain.ValidationErrors {
	validationErrors := a.Request.Validate()

	// a class already refused for its shape, here or in a service, is not
	// one to say anything further about.
	if refusedRuntime(validationErrors) {
		return validationErrors
	}

	class, agree := a.Stack.Class(a.classes.Default())

	switch {
	case !agree:
		validationErrors["runtime"] = "mixed_runtimes_in_stack"

	case !a.classes.Allows(class):
		validationErrors["runtime"] = "invalid_value"
	}

	return validationErrors
}

func refusedRuntime(validationErrors domain.ValidationErrors) bool {
	for field := range validationErrors {
		if field == "runtime" || strings.HasSuffix(field, ".runtime") {
			return true
		}
	}

	return false
}
