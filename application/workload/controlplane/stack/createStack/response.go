package createStack

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

// Response is the stack being deployed and the Docker VM it is in, or why
// there is none.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	VM    *presenter.ChosenVM `json:"vm,omitempty"`
	Stack *presenter.Stack    `json:"stack,omitempty"`
}
