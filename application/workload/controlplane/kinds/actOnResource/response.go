package actOnResource

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Response is what asking came to.
type Response struct {
	ValidationErrors domain.ValidationErrors

	// NodeError is why a command for a node could not be sent: the resource
	// is on no node yet.
	NodeError *noderequest.Error

	// Resource is the resource as it was left, unless it is Gone.
	Resource kind.Raw
	Gone     bool

	// Command is what its node was sent, for a command run on one, and
	// Result what came of it, when it was waited for and came in time.
	Command *kind.ActOnResource
	Result  *kind.ResourceActedOn

	// Dependents are the roots whose graphs need what a command that deletes
	// it was asked of, when it is not a root itself, as they were when it was
	// asked: those asked to go with it, when it was deleted with them
	// (Request.Cascade). Recreated says one of them makes it again once it is
	// gone, which none does once they go with it.
	Dependents []graph.Dependent
	Recreated  bool
}
