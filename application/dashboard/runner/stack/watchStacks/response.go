package watchStacks

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

// ChangeResponse is what became of one stack, as the dashboard shows it. A
// stack that is gone is reported by uuid alone, because there is nothing left
// to describe.
type ChangeResponse struct {
	Kind  string           `json:"kind"`
	UUID  string           `json:"uuid"`
	Stack *presenter.Stack `json:"stack,omitempty"`
}
