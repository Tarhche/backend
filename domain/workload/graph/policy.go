package graph

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

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

// With is p with what over gives laid over it, entry by entry: each kind and
// each pair over gives a type is given that type, and every other keeps what
// p gives it, so that a policy configured over the defaults changes only what
// it names. What over gives that is no type it does not give, and changes
// nothing. Neither policy is changed.
func (p DependencyPolicy) With(over DependencyPolicy) DependencyPolicy {
	laid := DependencyPolicy{ByPair: maps.Clone(p.ByPair), ByKind: maps.Clone(p.ByKind)}

	for pair, t := range over.ByPair {
		if t.IsValid() {
			laid.ByPair = give(laid.ByPair, pair, t)
		}
	}

	for kindName, t := range over.ByKind {
		if t.IsValid() {
			laid.ByKind = give(laid.ByKind, kindName, t)
		}
	}

	return laid
}

// String is the policy as ParsePolicy reads it: each kind it gives a type,
// and then each pair, in the order of their names. What it gives that is no
// type it does not give (Type), and is not said. An order, which a policy is
// given in code alone, is said, as no entry ParsePolicy reads.
func (p DependencyPolicy) String() string {
	var entries []string

	for _, kindName := range slices.Sorted(maps.Keys(p.ByKind)) {
		if t := p.ByKind[kindName]; t.IsValid() {
			entries = append(entries, kindName+"="+t.String())
		}
	}

	pairs := slices.SortedFunc(maps.Keys(p.ByPair), func(a, b [2]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})

	for _, pair := range pairs {
		if t := p.ByPair[pair]; t.IsValid() {
			entries = append(entries, pair[0]+"/"+pair[1]+"="+t.String())
		}
	}

	return strings.Join(entries, ",")
}

// ParsePolicy is the policy text says, as a control plane is configured with
// one: entries separated by commas, each a kind and the type of every edge to
// it, kind=owned or kind=loose, or a pair of kinds and the type of the edge
// between them, dependent/required=owned or loose, as stack/volume=owned.
// Space around a part of an entry is not part of it, and a text of nothing is
// a policy that gives nothing.
//
// An entry that is neither is an error naming it, and so is one that names
// no kind, or what is no kind's name (a word), that gives what is no type, or
// that gives a kind or a pair a type an entry before it gave. So is one that
// gives an order: an edge is an order only because a node is made after
// another (depends_on), never because a policy says so.
func ParsePolicy(text string) (DependencyPolicy, error) {
	var p DependencyPolicy

	if len(strings.TrimSpace(text)) == 0 {
		return p, nil
	}

	for i, entry := range strings.Split(text, ",") {
		entry = strings.TrimSpace(entry)
		if len(entry) == 0 {
			return DependencyPolicy{}, fmt.Errorf("entry %d is empty", i+1)
		}

		key, value, ok := strings.Cut(entry, "=")
		names := strings.Split(key, "/")

		if !ok || len(names) > 2 {
			return DependencyPolicy{}, fmt.Errorf("entry %q is neither kind=type nor dependent/required=type", entry)
		}

		for j := range names {
			names[j] = strings.TrimSpace(names[j])

			switch {
			case len(names[j]) == 0:
				return DependencyPolicy{}, fmt.Errorf("entry %q names no kind", entry)
			case !kind.IsWord(names[j]):
				return DependencyPolicy{}, fmt.Errorf("entry %q names %q, which is no kind: a kind's name is a lowercase word", entry, names[j])
			}
		}

		t, err := typeOf(strings.TrimSpace(value))
		if err != nil {
			return DependencyPolicy{}, fmt.Errorf("entry %q %w", entry, err)
		}

		if len(names) == 1 {
			if _, given := p.ByKind[names[0]]; given {
				return DependencyPolicy{}, fmt.Errorf("entry %q gives %s a type an entry before it gave", entry, names[0])
			}

			p.ByKind = give(p.ByKind, names[0], t)

			continue
		}

		pair := [2]string{names[0], names[1]}
		if _, given := p.ByPair[pair]; given {
			return DependencyPolicy{}, fmt.Errorf("entry %q gives %s/%s a type an entry before it gave", entry, pair[0], pair[1])
		}

		p.ByPair = give(p.ByPair, pair, t)
	}

	return p, nil
}

// typeOf is the type an entry of a policy gives, as it says it: owned or
// loose. Anything else is an error saying what it gives.
func typeOf(said string) (EdgeType, error) {
	switch said {
	case Owned.String():
		return Owned, nil
	case Loose.String():
		return Loose, nil
	case Order.String():
		return 0, errors.New("gives an order, which only depends_on does")
	}

	return 0, fmt.Errorf("gives %q, which is no type: an edge is owned or loose", said)
}

// give is given with what key names given the type t: given itself, or a map
// of its own when there is none.
func give[K comparable](given map[K]EdgeType, key K, t EdgeType) map[K]EdgeType {
	if given == nil {
		given = make(map[K]EdgeType)
	}

	given[key] = t

	return given
}
