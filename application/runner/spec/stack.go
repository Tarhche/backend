package spec

import (
	"regexp"

	"github.com/khanzadimahdi/testproject/domain"
)

// serviceName is the shape a service's name has to take: it becomes a network
// alias its neighbours reach it by, so it has to be a hostname.
var serviceName = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$`)

// maxServices caps one stack, so a single request cannot ask a node for an
// unbounded number of containers.
const maxServices = 20

// Stack is a set of services run together, in the shape a compose file has.
// The services share a private network and reach each other by the names they
// are keyed under, exactly as they would under compose.
type Stack struct {
	Name     string             `json:"name"`
	Services map[string]Service `json:"services"`
}

// Validate reports what is wrong with a stack, under the field names the client
// sent, so an error points at the service it came from.
func (s *Stack) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(s.Name) == 0 {
		validationErrors["name"] = "required_field"
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

		for field, message := range service.Validate("services." + name) {
			validationErrors[field] = message
		}
	}

	return validationErrors
}
