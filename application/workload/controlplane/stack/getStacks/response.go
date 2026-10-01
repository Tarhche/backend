package getStacks

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/internal/report"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

type Response struct {
	Items      []report.Stack `json:"items"`
	Pagination Pagination     `json:"pagination"`
}

type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(stacks []stack.Stack, services map[string][]task.Task, totalPages uint, currentPage uint) *Response {
	items := make([]report.Stack, len(stacks))
	for i, s := range stacks {
		items[i] = report.NewStack(s, services[s.UUID])
	}

	return &Response{
		Items: items,
		Pagination: Pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
