package createSnapshot

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

// Response is the snapshot being taken, or why it is not.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Snapshot *presenter.Snapshot `json:"snapshot,omitempty"`
}
