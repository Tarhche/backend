package getVMs

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Request struct {
	Page uint `json:"page"`

	// Kind narrows the listing to one kind: docker lists the VMs containers
	// and stacks can go in.
	Kind string `json:"kind"`

	// OwnerUUID narrows the listing to one person's own, and is empty for
	// everybody's.
	OwnerUUID string `json:"-"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Kind) > 0 && !vm.Kind(r.Kind).IsValid() {
		validationErrors["kind"] = "invalid_kind"
	}

	return validationErrors
}
