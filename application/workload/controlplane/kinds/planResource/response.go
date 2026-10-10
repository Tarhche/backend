package planResource

import (
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Response is what admitting or deleting a resource would do, read and not
// done.
type Response struct {
	// ValidationErrors is why it would not be admitted, as its admission
	// would refuse it.
	ValidationErrors domain.ValidationErrors

	// Resource is what would be admitted, as it would be kept, under a uuid
	// nothing holds: nothing is.
	Resource kind.Raw

	// Plan is what carrying out the graph of a root would do: of one to
	// admit, what making it would make, find there or wait on, level by
	// level; of one to delete, what removing it would remove, unlink and
	// keep.
	Plan *execute.Preview

	// Dependents are the roots whose graphs need what is not a root, which
	// a delete would be answered with, each with what removing it would do
	// when it would go with it (Request.Cascade); and Recreated whether one
	// of them would make it again, which none would once they go with it.
	Dependents []Dependent
	Recreated  bool
}

// Dependent is a root whose graph needs what would be deleted, and what
// removing it would do, when it would go with it.
type Dependent struct {
	graph.Dependent

	Plan *execute.Preview
}
