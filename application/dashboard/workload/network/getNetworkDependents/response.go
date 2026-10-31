package getNetworkDependents

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Needs are the roots that need the network, nothing owning them: one on
	// it, which would have its delete refused, or one that makes it again
	// once it is gone, as its delete would be answered with them now.
	presenter.Needs
}
