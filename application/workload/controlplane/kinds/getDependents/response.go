package getDependents

import "github.com/khanzadimahdi/testproject/domain/workload/graph"

// Response is which roots need a resource, as its delete would be answered
// with them now.
type Response struct {
	// Dependents are the roots whose graphs need it, those not asked to go,
	// each with whether its graph makes it again once it is gone: none for a
	// root, which no other root needs, and none for what nobody keeps a
	// record of.
	Dependents []graph.Dependent

	// Recreated says one of them makes it again once it is gone.
	Recreated bool
}
