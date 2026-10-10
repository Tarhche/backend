package graph

import "github.com/hmdsefi/gograph"

// Graph is the graph a plan keeps its nodes and edges in, for a test to hold
// it to having each node as one vertex.
func (p *Plan) Graph() gograph.Graph[NodeID] {
	return p.graph
}
