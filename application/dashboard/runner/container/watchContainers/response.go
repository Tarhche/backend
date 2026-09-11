package watchContainers

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

// ChangeResponse is what became of one container, as the dashboard shows it. A
// container that is gone is reported by uuid alone, because there is nothing
// left to describe.
type ChangeResponse struct {
	Kind      string               `json:"kind"`
	UUID      string               `json:"uuid"`
	Container *presenter.Container `json:"container,omitempty"`
}
