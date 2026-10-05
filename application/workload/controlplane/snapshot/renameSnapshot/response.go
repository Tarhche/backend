package renameSnapshot

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

// Response is the snapshot as it now is, or why it was not renamed.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Snapshot *presenter.Snapshot `json:"snapshot,omitempty"`
}
