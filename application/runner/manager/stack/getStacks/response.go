package getStacks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Response struct {
	Items      []StackResponse `json:"items"`
	Pagination Pagination      `json:"pagination"`
}

type StackResponse struct {
	UUID      string    `json:"uuid"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	State     string    `json:"state"`
	Services  uint      `json:"services"`
	CreatedAt time.Time `json:"created_at"`
}

type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(stacks []stack.Stack, services map[string][]task.Task, totalPages uint, currentPage uint) *Response {
	items := make([]StackResponse, len(stacks))
	for i, s := range stacks {
		items[i] = StackResponse{
			UUID:      s.UUID,
			Name:      s.Name,
			Slug:      s.Slug,
			State:     stack.State(services[s.UUID]).String(),
			Services:  uint(len(services[s.UUID])),
			CreatedAt: s.CreatedAt,
		}
	}

	return &Response{
		Items: items,
		Pagination: Pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
