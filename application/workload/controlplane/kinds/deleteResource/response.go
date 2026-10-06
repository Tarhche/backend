package deleteResource

import (
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Response is what asking for a resource to be deleted came to.
type Response struct {
	// Resource is the resource as it was left, unless it is Gone: deleted
	// already, or with nothing of it anywhere to delete.
	Resource kind.Raw
	Gone     bool

	// Command is the delete its node was sent, and Result what came of it,
	// when it was waited for and came in time. Neither is there when it is
	// being deleted already, or cannot be asked yet: deleting it is what is
	// expected of it, and it is asked for once it can be.
	Command *kind.Command
	Result  *kind.Result
}
