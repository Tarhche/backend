package graph

// EdgeType is what deleting a dependent does to what it needs through an
// edge: whether the edge owns it, only uses it, or only orders the two.
type EdgeType int

const (
	// Owned is what a dependent owns: it goes once the last of what owns it
	// goes.
	Owned EdgeType = 1

	// Loose is what a dependent uses and does not own: it stays when the
	// dependent goes, as a VM outlives a stack in it.
	Loose EdgeType = 2

	// Order is the order of two nodes that need nothing of each other, the
	// one made before the other: a service and one it depends_on. Nothing is
	// owned through it.
	Order EdgeType = 3
)

// IsValid reports whether t is one of the types an edge has.
func (t EdgeType) IsValid() bool {
	return t == Owned || t == Loose || t == Order
}

func (t EdgeType) String() string {
	switch t {
	case Owned:
		return "owned"
	case Loose:
		return "loose"
	case Order:
		return "order"
	}

	return "unknown"
}

// Edge is one node needing another: From is made before To, and removed
// after it.
type Edge struct {
	From NodeID
	To   NodeID
	Type EdgeType
}

// DependencyPolicy is the type of the edge between two kinds: a kind says
// what it needs, and whether it owns it is the policy's to say, so that it is
// configured rather than written into a kind.
type DependencyPolicy struct {
	// ByPair is the type of an edge between a pair of kinds, by the
	// dependent's kind and the kind it requires, in that order: a stack's
	// containers, (stack, container).
	ByPair map[[2]string]EdgeType

	// ByKind is the type of every edge to a kind it requires, whatever
	// requires it.
	ByKind map[string]EdgeType
}

// DefaultPolicy is the policy nothing configured otherwise: a VM and a volume
// are loose, and outlive what needs them, and a network and a container are
// owned, and go with the last of what owns them, as any other kind is.
func DefaultPolicy() DependencyPolicy {
	// the kinds by their names, which are words that do not change
	// (kind.Check).
	return DependencyPolicy{ByKind: map[string]EdgeType{
		"vm":        Loose,
		"volume":    Loose,
		"network":   Owned,
		"container": Owned,
	}}
}

// Type is the type of the edge from a resource of childKind to one of
// parentKind that requires it: what the pair is given, or else what the
// required kind is given, or else Owned. What is given that is no type is
// not given.
func (p DependencyPolicy) Type(parentKind string, childKind string) EdgeType {
	if t, given := p.ByPair[[2]string{parentKind, childKind}]; given && t.IsValid() {
		return t
	}

	if t, given := p.ByKind[childKind]; given && t.IsValid() {
		return t
	}

	return Owned
}
