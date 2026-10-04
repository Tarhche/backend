package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

const StackFailedName = "workloadStackFailed"

// StackFailed says a compose command on a stack failed. Reason is why, and
// Output what compose printed before it gave up, which is usually the more
// useful of the two.
type StackFailed struct {
	StackUUID string       `json:"stack_uuid"`
	NodeName  string       `json:"node_name"`
	Action    stack.Action `json:"action"`
	Reason    string       `json:"reason"`
	Output    string       `json:"output"`
	At        time.Time    `json:"at"`
}
