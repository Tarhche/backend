package execute

import (
	"context"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Verdict is what carrying a plan out would do with a node of it.
type Verdict string

const (
	// Exists is a node there already, which applying takes as it is.
	Exists Verdict = "exists"

	// Create is a node applying would make.
	Create Verdict = "create"

	// Wait is an external node that is missing, which applying would wait
	// on, starting nothing after its level.
	Wait Verdict = "wait"

	// Delete is a node removing would remove: the root, or what it owns that
	// nothing that stays owns.
	Delete Verdict = "delete"

	// Unlink is a node removing would keep, as something that stays owns it,
	// and would take what it removes out of what owns it.
	Unlink Verdict = "unlink"

	// Keep is a node removing would never touch: one the root uses loosely,
	// is only made after, or that is external, and anything else it does not
	// own.
	Keep Verdict = "keep"

	// Gone is a node removing would remove, and finds gone already: there is
	// no record of it.
	Gone Verdict = "gone"
)

// Preview is what carrying a plan out would do, as its nodes are now, level
// by level: which are there already, which applying would make, and which
// external ones, missing, it would wait on; or which removing would remove,
// unlink or keep, and which are gone already.
type Preview struct {
	Levels [][]Previewed `json:"levels"`
}

// Previewed is a node of a plan, as carrying it out would find it.
type Previewed struct {
	Node    graph.NodeID `json:"node"`
	Kind    string       `json:"kind"`
	Verdict Verdict      `json:"verdict"`

	// External is a node made by somebody else, which is never made or
	// removed here.
	External bool `json:"external,omitempty"`

	// Ref is the record it is, when there is one, made or not.
	Ref graph.Ref `json:"ref,omitzero"`
}

// Preview is what applying plan would do, read and not done: each node is
// looked for, and nothing is made or saved. A plan with a node of a kind
// the executor is not given is kind.ErrUnknownKind, and what looking for a
// node fails with is an error naming it.
func (e *Executor) Preview(ctx context.Context, plan *graph.Plan) (Preview, error) {
	levels, err := e.levelsOf(plan)
	if err != nil {
		return Preview{}, err
	}

	preview := Preview{Levels: make([][]Previewed, 0, len(levels))}

	for _, ids := range levels {
		level := make([]Previewed, 0, len(ids))

		for _, id := range ids {
			node, _ := plan.Node(id)

			ref, there, err := e.kinds[node.Kind].Exists(ctx, node.Spec)
			if err != nil {
				return Preview{}, fmt.Errorf("whether %s is there: %w", id, err)
			}

			previewed := Previewed{Node: id, Kind: node.Kind, External: node.External, Ref: ref}

			switch {
			case there:
				previewed.Verdict = Exists
			case node.External:
				previewed.Verdict = Wait
			default:
				previewed.Verdict = Create
			}

			level = append(level, previewed)
		}

		preview.Levels = append(preview.Levels, level)
	}

	return preview, nil
}

// PreviewRemove is what removing plan would do, read and not done: each node
// it would remove is looked for, and what owns it, and nothing is removed,
// unlinked or saved. Its levels are the levels removing goes in: first those
// it would keep and unlink, and then, from the top down, those it would
// remove and those gone already; and last, a level of their own, those it
// would never touch. A plan with a node of a kind the executor is not given
// is kind.ErrUnknownKind, and what looking for a node, or for what owns it,
// fails with is an error naming it.
func (e *Executor) PreviewRemove(ctx context.Context, plan *graph.Plan) (Preview, error) {
	if _, err := e.levelsOf(plan); err != nil {
		return Preview{}, err
	}

	d, err := e.deletion(ctx, plan)
	if err != nil {
		return Preview{}, err
	}

	levels := d.levels()

	preview := Preview{Levels: make([][]Previewed, 0, len(levels)+1)}

	for _, ids := range levels {
		level := make([]Previewed, 0, len(ids))

		for _, id := range ids {
			node, _ := plan.Node(id)

			previewed := Previewed{Node: id, Kind: node.Kind, External: node.External, Ref: d.records[id]}

			_, kept := d.kept[id]
			_, recorded := d.records[id]

			switch {
			case kept:
				previewed.Verdict = Unlink
			case !recorded:
				previewed.Verdict = Gone
			default:
				previewed.Verdict = Delete
			}

			level = append(level, previewed)
		}

		preview.Levels = append(preview.Levels, level)
	}

	var untouched []Previewed

	for _, node := range plan.Nodes() {
		if !slices.Contains(d.candidates, node.ID()) {
			untouched = append(untouched, Previewed{Node: node.ID(), Kind: node.Kind, Verdict: Keep, External: node.External})
		}
	}

	if len(untouched) > 0 {
		preview.Levels = append(preview.Levels, untouched)
	}

	return preview, nil
}
