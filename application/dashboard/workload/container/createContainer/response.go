package createContainer

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// VM is the Docker VM the container went into, and whether it was made
	// for it.
	VM        *presenter.ChosenVM  `json:"vm,omitempty"`
	Container *presenter.Container `json:"container,omitempty"`
}
