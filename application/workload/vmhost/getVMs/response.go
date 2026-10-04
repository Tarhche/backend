package getVMs

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type Response struct {
	ValidationErrors domain.ValidationErrors

	// VMs are never nil: none is an empty list.
	VMs []vm.VM
}
