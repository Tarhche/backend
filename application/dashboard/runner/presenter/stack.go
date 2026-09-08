package presenter

import (
	"time"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// Stack is a stack and the services in it, as the dashboard shows it.
type Stack struct {
	UUID      string      `json:"uuid"`
	Name      string      `json:"name"`
	Slug      string      `json:"slug"`
	State     string      `json:"state"`
	Services  []Container `json:"services"`
	CreatedAt time.Time   `json:"created_at"`
}

func NewStack(s runnerManager.Stack, ingressDomain string) Stack {
	return Stack{
		UUID:      s.UUID,
		Name:      s.Name,
		Slug:      s.Slug,
		State:     s.State.String(),
		Services:  NewContainers(s.Services, ingressDomain),
		CreatedAt: s.CreatedAt,
	}
}

func NewStacks(stacks []runnerManager.Stack, ingressDomain string) []Stack {
	items := make([]Stack, len(stacks))
	for i := range stacks {
		items[i] = NewStack(stacks[i], ingressDomain)
	}

	return items
}

// Pagination is where a listing sits in the whole.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}
