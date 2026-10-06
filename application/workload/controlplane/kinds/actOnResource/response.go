package actOnResource

import (
	"github.com/khanzadimahdi/testproject/domain"
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
	Command *kind.Command
	Result  *kind.Result
}
