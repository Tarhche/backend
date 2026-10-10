package execute

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Verdict is what applying a plan would do with a node of it.
type Verdict string

const (
	// Exists is a node there already, which applying takes as it is.
	Exists Verdict = "exists"

	// Create is a node applying would make.
	Create Verdict = "create"

	// Wait is an external node that is missing, which applying would wait
	// on, starting nothing after its level.
	Wait Verdict = "wait"
)

// Preview is what applying a plan would do, as its nodes are now, level by
// level: which are there already, which it would make, and which external
// ones, missing, it would wait on.
type Preview struct {
	Levels [][]Previewed `json:"levels"`
}

// Previewed is a node of a plan, as applying it would find it.
type Previewed struct {
	Node    graph.NodeID `json:"node"`
	Kind    string       `json:"kind"`
	Verdict Verdict      `json:"verdict"`

	// External is a node made by somebody else, which is never made here.
	External bool `json:"external,omitempty"`

	// Ref is the record it is, when it is there.
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

			previewed := Previewed{Node: id, Kind: node.Kind, External: node.External}

			switch {
			case there:
				previewed.Verdict = Exists
				previewed.Ref = ref
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
