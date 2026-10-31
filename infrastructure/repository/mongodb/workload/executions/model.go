package executions

import (
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// document is an execution as it is stored. Nothing is omitted but its _id,
// which a save looks the document up by and never sets, since it never
// changes: whatever it is set to is written, empty or not, and what a save
// leaves empty is so.
type document struct {
	ID         string     `bson:"_id,omitempty"`
	Root       reference  `bson:"root"`
	Generation int64      `bson:"generation"`
	Node       string     `bson:"node"`
	Kind       string     `bson:"kind"`
	Action     string     `bson:"action"`
	Level      int        `bson:"level"`
	State      string     `bson:"state"`
	Reason     string     `bson:"reason"`
	Ref        *reference `bson:"ref"`
	StartedAt  *time.Time `bson:"started_at"`
	FinishedAt *time.Time `bson:"finished_at"`
	UpdatedAt  *time.Time `bson:"updated_at"`
}

type reference struct {
	Kind string `bson:"kind"`
	UUID string `bson:"uuid"`
}

// idOf is what an execution is stored under: its root's uuid and its node,
// "<root uuid>|<node>". A uuid has no bar, so the first one ends it.
func idOf(e graph.Execution) (string, error) {
	if len(e.Root.UUID) == 0 || len(e.Node) == 0 {
		return "", fmt.Errorf("an execution is kept by its root's uuid and its node, and that of %q in %q lacks one", e.Node, e.Root.UUID)
	}

	return e.Root.UUID + "|" + e.Node.String(), nil
}

// toDocument is what a save sets of an execution: everything but its _id.
func toDocument(e graph.Execution) document {
	stored := document{
		Root:       reference(e.Root),
		Generation: e.Generation,
		Node:       e.Node.String(),
		Kind:       e.Kind,
		Action:     string(e.Action),
		Level:      e.Level,
		State:      string(e.State),
		Reason:     e.Reason,
		StartedAt:  optional(e.StartedAt),
		FinishedAt: optional(e.FinishedAt),
		UpdatedAt:  optional(e.UpdatedAt),
	}

	if e.Ref != (graph.Ref{}) {
		ref := reference(e.Ref)
		stored.Ref = &ref
	}

	return stored
}

func toExecution(stored document) graph.Execution {
	e := graph.Execution{
		Root:       kind.Reference(stored.Root),
		Generation: stored.Generation,
		Node:       graph.NodeID(stored.Node),
		Kind:       stored.Kind,
		Action:     graph.Action(stored.Action),
		Level:      stored.Level,
		State:      graph.State(stored.State),
		Reason:     stored.Reason,
		StartedAt:  moment(stored.StartedAt),
		FinishedAt: moment(stored.FinishedAt),
		UpdatedAt:  moment(stored.UpdatedAt),
	}

	if stored.Ref != nil {
		e.Ref = kind.Reference(*stored.Ref)
	}

	return e
}

// optional is a time to store, to the millisecond and in UTC, as the
// database keeps it, or nothing for none.
func optional(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	kept := t.UTC().Truncate(time.Millisecond)

	return &kept
}

// moment is a stored time, in UTC, or none for nothing stored.
func moment(t *time.Time) time.Time {
	if t == nil || t.IsZero() {
		return time.Time{}
	}

	return t.UTC()
}
