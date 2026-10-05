package updateVM

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

// Response is the VM as it now is, or why it was not changed.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	VM *presenter.VM `json:"vm,omitempty"`
}
