package getStack

import (
	"github.com/khanzadimahdi/testproject/application/runner/manager/stack/internal/report"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Response is a stack and the services in it.
type Response = report.Stack

func NewResponse(s stack.Stack, services []task.Task) *Response {
	response := report.NewStack(s, services)

	return &response
}
