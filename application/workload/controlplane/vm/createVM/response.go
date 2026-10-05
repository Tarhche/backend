package createVM

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

// Response is the VM that was created, or why it was not.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	VM *presenter.VM `json:"vm,omitempty"`
}
