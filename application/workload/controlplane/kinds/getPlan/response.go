package getPlan

import "github.com/khanzadimahdi/testproject/domain/workload/graph"

// Response is how a root's plan went, the last time it was carried out.
type Response struct {
	// Root is the root, and Generation the generation of it its record is at.
	Root       graph.Ref
	Generation int64

	// Executions are how each node of its plan went, in the order of their
	// levels, and then of their nodes: none before its plan was first
	// carried out.
	Executions []graph.Execution

	// Lease is how its plan went lately, as its lease keeps it: its failures
	// in a row, what the last of them failed with, and when it is carried out
	// again; nothing while it has none.
	Lease *graph.Lease
}
