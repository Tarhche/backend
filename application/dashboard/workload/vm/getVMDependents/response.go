package getVMDependents

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Needs are the containers and the stacks that need the VM, which make it
	// again once it is gone, as its delete would be answered with them now.
	presenter.Needs
}
