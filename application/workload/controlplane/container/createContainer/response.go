package createContainer

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Response is the container that was created and the Docker VM it is in, or
// why there is none. A VM made for it is there either way.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// NodeError is why dockerd did not create it.
	NodeError *noderequest.Error `json:"error,omitempty"`

	VM        *presenter.ChosenVM    `json:"vm,omitempty"`
	Container *noderequest.Container `json:"container,omitempty"`
}
