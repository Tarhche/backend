package watch

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
)

// the kinds of change a client is told about.
const (
	kindChanged = "changed"
	kindDeleted = "deleted"
)

// TaskChange is one message of a task watch: "changed" carries the
// task as the runner last reported it, "deleted" only the uuid of one that
// is gone.
type TaskChange struct {
	Kind string `json:"kind"`
	UUID string `json:"uuid"`
	Task *Task  `json:"task,omitempty"`
}

// Task is as much of a task as the runner reports about one: enough
// to keep a row that was listed up to date, not enough to draw one that was
// never listed. Whatever a message does not say is left out rather than sent
// empty, so a client merges what it is told onto what it already has.
type Task struct {
	UUID      string               `json:"uuid"`
	Name      string               `json:"name,omitempty"`
	Slug      string               `json:"slug,omitempty"`
	Kind      string               `json:"kind,omitempty"`
	State     string               `json:"state,omitempty"`
	Image     string               `json:"image,omitempty"`
	StackUUID string               `json:"stack_uuid,omitempty"`
	NodeName  string               `json:"node_name,omitempty"`
	Attempt   int                  `json:"attempt,omitempty"`
	Reason    string               `json:"reason,omitempty"`
	Endpoints []presenter.Endpoint `json:"endpoints,omitempty"`
	Deadline  *time.Time           `json:"deadline,omitempty"`
	At        time.Time            `json:"at"`
}

// StackChange is one message of a stack watch. A stack has no report of its
// own — its state is read off its services — so it is read whole from the
// runner whenever one of them changes, exactly as a stack asked for on its own
// is.
type StackChange struct {
	Kind  string           `json:"kind"`
	UUID  string           `json:"uuid"`
	Stack *presenter.Stack `json:"stack,omitempty"`
}
