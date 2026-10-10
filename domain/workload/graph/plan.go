package graph

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hmdsefi/gograph"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Planner makes the plan of what a root needs (Plan), through what the kind
// of each of its nodes says it requires, with each edge typed by the policy.
// It reads nothing but the requirements it is handed and what the kinds say
// of them, and asks the kinds nothing else.
type Planner struct {
	kinds  Kinds
	policy DependencyPolicy
}

// NewPlanner is a planner of graphs made of kinds, whose edges policy types.
func NewPlanner(kinds Kinds, policy DependencyPolicy) *Planner {
	return &Planner{kinds: kinds, policy: policy}
}

// Plan is the plan of what root needs: root, what its kind says it requires,
// what theirs say they require, and so on down to what requires nothing,
// breadth first, each node asked what it requires once, however many need it.
//
// Each requirement is an edge from the node required to the node requiring
// it, typed by the policy by the kinds of the two; once every node is in the
// plan, each node a node is made after is an edge to it too, an order. An
// edge that is there already keeps the stronger of its types: owned, then
// loose, then order.
//
// A node of a kind the planner is not given is kind.ErrUnknownKind, and a
// node made after one the plan does not have is an error naming both. Nodes
// made after one another in a ring are a *Cycle. Nodes that require one
// another in a ring are an error naming them too, but not a Cycle: no kind
// may say so, and it is a kind's mistake, not what anybody asked for.
func (p *Planner) Plan(root Requirement) (*Plan, error) {
	plan := &Plan{
		root:  root.ID(),
		graph: gograph.New[NodeID](gograph.Acyclic()),
		nodes: make(map[NodeID]*Node),
		edges: make(map[[2]NodeID]EdgeType),
	}

	if _, _, ok := plan.root.Split(); !ok {
		return nil, fmt.Errorf("the root %q names no node", plan.root)
	}

	plan.add(root)

	// breadth first, from the root: a node is asked what it requires as the
	// first requirement of it says it, and only once.
	queue := []NodeID{plan.root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		node := plan.nodes[id]

		k, known := p.kinds[node.Kind]
		if !known {
			return nil, fmt.Errorf("%w: %q, the kind of %s", kind.ErrUnknownKind, node.Kind, id)
		}

		required, err := k.Requires(node.Spec)
		if err != nil {
			return nil, fmt.Errorf("what %s requires: %w", id, err)
		}

		for _, r := range required {
			needed := r.ID()
			if _, _, ok := needed.Split(); !ok {
				return nil, fmt.Errorf("%s requires %q, which names no node", id, needed)
			}

			if seen, ok := plan.nodes[needed]; ok {
				seen.merge(r)
			} else {
				plan.add(r)
				queue = append(queue, needed)
			}

			err = plan.join(needed, id, p.policy.Type(node.Kind, r.Kind))
			switch {
			case errors.Is(err, gograph.ErrDAGCycle):
				return nil, fmt.Errorf("requirement cycle: %s", ring(plan.cycle(needed, id)))
			case err != nil:
				return nil, err
			}
		}
	}

	// once every node is in the plan, each is made after what it says it is
	// made after, both in the order of their ids, so that a ring is found
	// and said the same way whatever order the kinds said them in.
	for _, id := range slices.Sorted(maps.Keys(plan.nodes)) {
		for _, after := range slices.Sorted(slices.Values(plan.nodes[id].After)) {
			if _, ok := plan.nodes[after]; !ok {
				return nil, fmt.Errorf("%s is made after %s, which is not in the plan", id, after)
			}

			err := plan.join(after, id, Order)
			switch {
			case errors.Is(err, gograph.ErrDAGCycle):
				return nil, &Cycle{Nodes: plan.cycle(after, id)}
			case err != nil:
				return nil, err
			}
		}
	}

	return plan, nil
}

// Plan is a root and what it needs, as a graph with no cycle (Planner.Plan):
// its nodes, each once, and its edges, each from a node needed to a node
// that needs it, with its type. It is made from the bottom up, a level at a
// time (Levels).
//
// It is not changed once it is made, so it may be read from several
// goroutines at once, and what it answers is a copy of its own.
type Plan struct {
	root NodeID

	// graph is the plan's nodes and its edges, each vertex labelled with its
	// node's id, which carries nothing else: the nodes, and the edges' types,
	// are kept beside it by the same ids.
	graph gograph.Graph[NodeID]
	nodes map[NodeID]*Node
	edges map[[2]NodeID]EdgeType
}

// Node is one node of a plan, as the first requirement of it said it, but
// for what every requirement of it says together: it is made after every
// node any of them says it is made after, and is External only when all of
// them say it is, since one that does not names a resource the graph is to
// manage.
type Node struct {
	Requirement
}

// Cycle is nodes made after one another in a ring, which no order makes: a
// stack's services that depend_on one another, round to the first. A plan
// with one is refused with it, and errors.As finds it, however it is
// wrapped.
type Cycle struct {
	// Nodes are the ring, each once, from the node whose After closed it:
	// each is made after the next, and the last after the first.
	Nodes []NodeID
}

func (c *Cycle) Error() string {
	return "depends_on cycle: " + ring(c.Nodes)
}

// Root is the node the plan is made for.
func (p *Plan) Root() NodeID {
	return p.root
}

// Node is the node id names, and whether the plan has it.
func (p *Plan) Node(id NodeID) (Node, bool) {
	node, ok := p.nodes[id]
	if !ok {
		return Node{}, false
	}

	return node.clone(), true
}

// Nodes are every node of the plan, in the order of their ids.
func (p *Plan) Nodes() []Node {
	nodes := make([]Node, 0, len(p.nodes))
	for _, node := range p.nodes {
		nodes = append(nodes, node.clone())
	}

	slices.SortFunc(nodes, func(a, b Node) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	return nodes
}

// Edges are every edge of the plan, in the order of the nodes they are from,
// and then of those they are to.
func (p *Plan) Edges() []Edge {
	return p.edgesWhere(func(Edge) bool { return true })
}

// Dependencies are the edges to the node id names, in the order of the nodes
// they are from: what it needs, and whether it owns each, uses it, or is only
// made after it.
func (p *Plan) Dependencies(id NodeID) []Edge {
	return p.edgesWhere(func(e Edge) bool { return e.To == id })
}

// Dependents are the edges from the node id names, in the order of the nodes
// they are to: what needs it, and how.
func (p *Plan) Dependents(id NodeID) []Edge {
	return p.edgesWhere(func(e Edge) bool { return e.From == id })
}

// Edge is the edge from one node to another, which is made before it, and
// whether the plan has one.
func (p *Plan) Edge(from NodeID, to NodeID) (Edge, bool) {
	t, ok := p.edges[[2]NodeID{from, to}]
	if !ok {
		return Edge{}, false
	}

	return Edge{From: from, To: to, Type: t}, true
}

// Levels are the plan's nodes in the order they are made in, a level at a
// time: the first needs nothing, and each after it nothing but what the
// levels before it hold, so that the nodes of a level need nothing of one
// another and may be made at once. Each level is in the order of its nodes'
// ids, so that a plan is made in one order, however it was planned.
func (p *Plan) Levels() [][]NodeID {
	return p.levels(func(NodeID) bool { return true })
}

// Owned are the nodes the node id names owns, in the order of their ids: what
// it needs through an owned edge, what those need through one, and so on,
// which go when it goes. What it uses loosely, or is only made after, it does
// not own, nor anything reached through it; and an external node is never
// owned, since it is never made or removed through a graph, nor anything
// reached through one. A node the plan does not have owns nothing.
func (p *Plan) Owned(id NodeID) []NodeID {
	if _, ok := p.nodes[id]; !ok {
		return nil
	}

	owned := make(map[NodeID]bool)

	// from the node down, along the owned edges to each node reached.
	queue := []NodeID{id}
	for len(queue) > 0 {
		dependent := queue[0]
		queue = queue[1:]

		for edge, t := range p.edges {
			needed := edge[0]
			if edge[1] != dependent || t != Owned || owned[needed] || p.nodes[needed].External {
				continue
			}

			owned[needed] = true
			queue = append(queue, needed)
		}
	}

	return slices.Sorted(maps.Keys(owned))
}

// LevelsOf are the levels of the part of the plan the nodes ids name make,
// ordered by every edge among them, as Levels are of the whole plan: what
// removing a root removes, from the last level to the first, as a plan is
// made from the first to the last. An edge between two of them orders them
// whatever its type, a loose one too, so that what needs a node goes before
// it: a service goes before a volume it mounts, loosely, that its stack owns
// and removes too. An edge to a node not among them orders nothing, and a
// node the plan does not have is in none of them.
func (p *Plan) LevelsOf(ids []NodeID) [][]NodeID {
	in := make(map[NodeID]bool, len(ids))
	for _, id := range ids {
		if _, ok := p.nodes[id]; ok {
			in[id] = true
		}
	}

	return p.levels(func(id NodeID) bool { return in[id] })
}

// levels are the plan's nodes in holds of, a level at a time, ordered by
// every edge between them: Kahn's algorithm, the first level what waits on
// nothing, and each after it what waits on nothing but the levels before it,
// each in the order of its nodes' ids.
func (p *Plan) levels(in func(NodeID) bool) [][]NodeID {
	// how many nodes each node still waits on, and which wait on it.
	waits := make(map[NodeID]int, len(p.nodes))
	waiting := make(map[NodeID][]NodeID, len(p.nodes))
	for edge := range p.edges {
		if in(edge[0]) && in(edge[1]) {
			waits[edge[1]]++
			waiting[edge[0]] = append(waiting[edge[0]], edge[1])
		}
	}

	var level []NodeID
	for id := range p.nodes {
		if in(id) && waits[id] == 0 {
			level = append(level, id)
		}
	}

	var levels [][]NodeID
	for len(level) > 0 {
		slices.Sort(level)
		levels = append(levels, level)

		var next []NodeID
		for _, id := range level {
			for _, dependent := range waiting[id] {
				waits[dependent]--
				if waits[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}

		level = next
	}

	return levels
}

// add takes r into the plan as a node of its own, with a vertex of its own.
func (p *Plan) add(r Requirement) {
	first := r
	first.After = nil

	node := &Node{Requirement: first}
	node.merge(r)

	p.nodes[r.ID()] = node
	p.vertex(r.ID())
}

// vertex is the vertex of the node id names, added when the graph has none:
// every edge is added between the graph's own vertices, so that no node is
// ever a vertex twice.
func (p *Plan) vertex(id NodeID) *gograph.Vertex[NodeID] {
	if v := p.graph.GetVertexByID(id); v != nil {
		return v
	}

	return p.graph.AddVertexByLabel(id)
}

// join adds the edge from one node to another, typed t, or, when there is one
// already, keeps the stronger of its type and t. One that would close a ring
// is not added, and is gograph.ErrDAGCycle.
func (p *Plan) join(from NodeID, to NodeID, t EdgeType) error {
	_, err := p.graph.AddEdge(p.vertex(from), p.vertex(to))
	if err != nil && !errors.Is(err, gograph.ErrEdgeAlreadyExists) {
		return err
	}

	edge := [2]NodeID{from, to}
	if had, ok := p.edges[edge]; !ok || stronger(t, had) {
		p.edges[edge] = t
	}

	return nil
}

// cycle is the ring an edge from one node to another would close, through
// the shortest path the graph has back from to to from: its nodes, to first,
// each needing the next, and the last needing to. It is found the same way
// whatever order the edges were added in.
func (p *Plan) cycle(from NodeID, to NodeID) []NodeID {
	// breadth first from to, along what needs each node, in the order of
	// their ids.
	previous := map[NodeID]NodeID{to: to}
	queue := []*gograph.Vertex[NodeID]{p.vertex(to)}

	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]

		if v.Label() == from {
			break
		}

		neighbors := v.Neighbors()
		slices.SortFunc(neighbors, func(a, b *gograph.Vertex[NodeID]) int {
			return cmp.Compare(a.Label(), b.Label())
		})

		for _, neighbor := range neighbors {
			if _, seen := previous[neighbor.Label()]; !seen {
				previous[neighbor.Label()] = v.Label()
				queue = append(queue, neighbor)
			}
		}
	}

	if _, reached := previous[from]; !reached {
		return []NodeID{to, from}
	}

	// walked back from from, each node of the path needs the one before it.
	nodes := []NodeID{to}
	for id := from; id != to; id = previous[id] {
		nodes = append(nodes, id)
	}

	return nodes
}

// merge takes another requirement of the node into it: the node is made
// after what that says too, and is External only if that says so as well.
func (n *Node) merge(r Requirement) {
	n.External = n.External && r.External

	for _, after := range r.After {
		if !slices.Contains(n.After, after) {
			n.After = append(n.After, after)
		}
	}
}

// clone is the node as a copy of its own.
func (n *Node) clone() Node {
	c := *n
	c.After = slices.Clone(n.After)

	return c
}

// edgesWhere are the plan's edges match holds of, in the order of the nodes
// they are from, and then of those they are to.
func (p *Plan) edgesWhere(match func(Edge) bool) []Edge {
	var edges []Edge
	for edge, t := range p.edges {
		if e := (Edge{From: edge[0], To: edge[1], Type: t}); match(e) {
			edges = append(edges, e)
		}
	}

	slices.SortFunc(edges, func(a, b Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})

	return edges
}

// strengths are the types of an edge from the weakest to the strongest: an
// order holds nothing of what it orders, a loose edge uses what it needs, and
// an owned edge owns it.
var strengths = []EdgeType{Order, Loose, Owned}

// stronger reports whether an edge typed t holds what it needs more than one
// typed than does.
func stronger(t EdgeType, than EdgeType) bool {
	return slices.Index(strengths, t) > slices.Index(strengths, than)
}

// ring is nodes in a ring, said in order and back to the first.
func ring(nodes []NodeID) string {
	said := make([]string, 0, len(nodes)+1)
	for _, id := range nodes {
		said = append(said, id.String())
	}

	if len(nodes) > 0 {
		said = append(said, nodes[0].String())
	}

	return strings.Join(said, " → ")
}
