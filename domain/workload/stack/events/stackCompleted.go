package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

const StackCompletedName = "workloadStackCompleted"

// StackCompleted says a compose command on a stack succeeded, with what it
// printed.
type StackCompleted struct {
	StackUUID string       `json:"stack_uuid"`
	NodeName  string       `json:"node_name"`
	Action    stack.Action `json:"action"`
	Output    string       `json:"output"`
	At        time.Time    `json:"at"`
}
