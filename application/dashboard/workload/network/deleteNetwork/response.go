package deleteNetwork

import (
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Dependents are what needs it, the stacks and the containers nothing
	// owns, and Recreated says one of them makes it again: none does once
	// they go first (Request.Cascade). A refusal is said with them all the
	// same.
	Dependents []presenter.Dependent `json:"dependents,omitempty"`
	Recreated  bool                  `json:"recreated,omitempty"`
}
