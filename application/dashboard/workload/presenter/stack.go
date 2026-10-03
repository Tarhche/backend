package presenter

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// Stack is a stack and the services in it, as the dashboard shows it.
type Stack struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	State string `json:"state"`

	// Runtime is the one class every service of the stack runs with.
	Runtime string `json:"runtime"`

	// ExpectedState is what the stack was asked to be, which is what it is on
	// its way to while a command is still reaching its services.
	ExpectedState string    `json:"expected_state,omitempty"`
	Services      []Task    `json:"services"`
	CreatedAt     time.Time `json:"created_at"`

	// Owner is who asked for this stack.
	Owner Owner `json:"owner"`
}

func NewStack(s workloadControlPlane.Stack, ingressDomain string, owners Owners) Stack {
	return Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		Slug:          s.Slug,
		State:         s.State.String(),
		Runtime:       s.Runtime.OrSysbox().String(),
		ExpectedState: s.ExpectedState.String(),
		Services:      NewTasks(s.Services, ingressDomain, owners),
		CreatedAt:     s.CreatedAt,
		Owner:         owners.Of(s.OwnerUUID),
	}
}

func NewStacks(stacks []workloadControlPlane.Stack, ingressDomain string, owners Owners) []Stack {
	items := make([]Stack, len(stacks))
	for i := range stacks {
		items[i] = NewStack(stacks[i], ingressDomain, owners)
	}

	return items
}

// Pagination is where a listing sits in the whole.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}
