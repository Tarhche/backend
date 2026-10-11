package graph

import (
	"context"
	"time"
)

// Action is what carrying a plan out does to a node of it.
type Action string

const (
	// ActionCreate makes a node, from the bottom up, and ActionDelete
	// removes one, from the top down.
	ActionCreate Action = "create"
	ActionDelete Action = "delete"
)

// State is how far a node got, the last time its plan was carried out.
type State string

const (
	// StatePending is a node not started: its plan has not got to it, or
	// stopped before it did, or was cancelled while it ran.
	StatePending State = "pending"

	// StateRunning is a node being looked for, and made or removed.
	StateRunning State = "running"

	// StateWaiting is a node that waits on what it needs, which is there and
	// cannot take it now (Waiting), or on an external node that is missing.
	StateWaiting State = "waiting"

	// StateDone is a node its action was carried out on: there, as its plan
	// needs it, or gone.
	StateDone State = "done"

	// StateFailed is a node that failed, with what it failed with as its
	// reason.
	StateFailed State = "failed"
)

// Execution is how one node of a root's plan went, the last time the plan was
// carried out: what was to be done to it, how far it got, and why it is where
// it is.
type Execution struct {
	// Root is the record the plan is of, and Generation the generation of it
	// the plan was made from.
	Root       Ref
	Generation int64

	// Node is the node, and Kind its kind.
	Node NodeID
	Kind string

	// Action is what was to be done to it, and Level the level of the plan it
	// is in: the first, 0, needs nothing of the rest.
	Action Action
	Level  int

	// State is how far it got, and Reason why it is where it is: what it
	// waits on, or what it failed with, or what stopped it.
	State  State
	Reason string

	// Ref is the record it is, once that is known.
	Ref Ref

	// StartedAt is when it was started, and FinishedAt when it ended, done,
	// waiting or failed; UpdatedAt is when it was saved last.
	StartedAt  time.Time
	FinishedAt time.Time
	UpdatedAt  time.Time
}

// Executions keeps how each node of each root's plan went, the last time the
// plan was carried out and found anything to say: one execution for a node of
// a root, by the root's uuid and the node, whichever action it was of,
// written over each time it is saved. Only one control plane carries a root's
// plan out at a time, so what was saved last is what is so. A run that finds
// every node as the run before left it saves nothing.
type Executions interface {
	// Save keeps e in place of what was kept of its node of its root, every
	// field of it, those left empty too. One that names no root's uuid or no
	// node is an error, and is not kept.
	Save(ctx context.Context, e Execution) error

	// GetByRoot is every execution kept of the root uuid names, in the order
	// of their levels, and then of their nodes: none of a root of which
	// nothing is kept.
	GetByRoot(ctx context.Context, rootUUID string) ([]Execution, error)

	// DeleteByRoot forgets every execution kept of the root uuid names, once
	// the root is gone. One of which nothing is kept is forgotten already.
	DeleteByRoot(ctx context.Context, rootUUID string) error

	// DeleteBefore forgets every execution kept of the root uuid names of a
	// generation before generation: once every node of the plan of that
	// generation is saved, which saves each at it, those of nodes the plan
	// no longer has. Those of that generation, or of a later one, are kept.
	DeleteBefore(ctx context.Context, rootUUID string, generation int64) error
}
