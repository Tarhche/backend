package getStack

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/internal/report"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Response is a stack and the services in it.
type Response = report.Stack

func NewResponse(s stack.Stack, services []task.Task) *Response {
	response := report.NewStack(s, services)

	return &response
}
