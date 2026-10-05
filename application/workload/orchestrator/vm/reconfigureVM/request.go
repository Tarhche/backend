package reconfigureVM

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Request asks this node to give one of its VMs the ports, network and
// resources its spec now says.
type Request struct {
	VMUUID string  `json:"vm_uuid"`
	Spec   vm.Spec `json:"spec"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	if !r.Spec.Network.Ingress.IsValid() {
		validationErrors["spec.network.ingress"] = "invalid_value"
	}

	if !r.Spec.Network.Egress.IsValid() {
		validationErrors["spec.network.egress"] = "invalid_value"
	}

	return validationErrors
}
