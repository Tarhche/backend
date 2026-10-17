package getVolumeDependents

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Needs are the roots that need the volume, nothing owning them: one that
	// mounts it, which would have its delete refused, as its delete would be
	// answered with them now.
	presenter.Needs
}
