package spec

import (
	"maps"
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// serviceName is the shape a service's name has to take: it becomes a network
// alias its neighbours reach it by, so it has to be a hostname.
var serviceName = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$`)

// maxServices caps one stack, so a single request cannot ask a node for an
// unbounded number of tasks.
const maxServices = 20

// Stack is a set of services run together, in the shape a compose file has.
// The services share a private network and reach each other by the names they
// are keyed under, exactly as they would under compose.
type Stack struct {
	Name string `json:"name"`

	// Runtime is the class every service that names none is run with.
	//
	// A stack is run as one class or not at all: its services share a
	// network on one node, and a network belongs to the one class that made
	// it. So a class can be named once, here, for the whole stack, and a
	// service naming its own has to name the same one.
	Runtime runtime.Class `json:"runtime,omitempty"`

	Services map[string]Service `json:"services"`
}

// Validate reports what is wrong with a stack, under the field names the client
// sent, so an error points at the service it came from.
func (s *Stack) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(s.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(s.Runtime) > 0 && !s.Runtime.IsValid() {
		validationErrors["runtime"] = "invalid_value"
	}

	if len(s.Services) == 0 {
		validationErrors["services"] = "required_field"

		return validationErrors
	}

	if len(s.Services) > maxServices {
		validationErrors["services"] = "too_many_services"

		return validationErrors
	}

	for name, service := range s.Services {
		if !serviceName.MatchString(name) {
			validationErrors["services."+name] = "invalid_value"

			continue
		}

		maps.Copy(validationErrors, service.Validate("services."+name))
	}

	// two services naming two classes cannot be one stack whatever the
	// default is. One naming a class and another naming none can be, if the
	// default is that class, and only the workload knows its default: it
	// asks again, with Class, once it has filled it in.
	if _, refused := validationErrors["runtime"]; !refused && len(s.namedClasses()) > 1 {
		validationErrors["runtime"] = "mixed_runtimes_in_stack"
	}

	return validationErrors
}

// ClassOf is the class one of the stack's services asks for: its own, or the
// stack's when it names none. Empty is neither, which leaves it to the
// workload's default.
func (s *Stack) ClassOf(service Service) runtime.Class {
	if len(service.Runtime) > 0 {
		return service.Runtime
	}

	return s.Runtime
}

// Class is the one class the stack is run with, once a service that asks for
// none is given the default. agree is false when its services would end up
// with more than one, which a stack cannot be run as.
func (s *Stack) Class(defaultClass runtime.Class) (class runtime.Class, agree bool) {
	resolve := func(asked runtime.Class) runtime.Class {
		if len(asked) == 0 {
			return defaultClass
		}

		return asked
	}

	// a stack with no services yet is what it names.
	class = resolve(s.Runtime)
	first := true

	for _, service := range s.Services {
		asked := resolve(s.ClassOf(service))

		if first {
			class, first = asked, false

			continue
		}

		if asked != class {
			return "", false
		}
	}

	return class, true
}

// namedClasses are the classes the stack's services ask for by name, each
// once. A name that cannot be a class is reported where it was written rather
// than counted here.
func (s *Stack) namedClasses() map[runtime.Class]struct{} {
	named := make(map[runtime.Class]struct{}, 1)

	for _, service := range s.Services {
		class := s.ClassOf(service)
		if len(class) == 0 || !class.IsValid() {
			continue
		}

		named[class] = struct{}{}
	}

	return named
}
