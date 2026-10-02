package stacks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

type StackBson struct {
	UUID string `bson:"_id,omitempty"`
	Name string `bson:"name"`
	Slug string `bson:"slug"`

	// Runtime is the one class the stack's services run with. A stack stored
	// before there were classes has none, and is read as sysbox, which is what
	// ran it; the backfill-runtime migration writes that down.
	Runtime string `bson:"runtime,omitempty"`

	ExpectedState int       `bson:"expected_state,omitempty"`
	NodeName      string    `bson:"node_name,omitempty"`
	OwnerUUID     string    `bson:"owner_uuid"`
	CreatedAt     time.Time `bson:"created_at,omitempty"`
}

func toStack(s *StackBson) stack.Stack {
	return stack.Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		Slug:          s.Slug,
		Runtime:       runtime.Class(s.Runtime).OrSysbox(),
		ExpectedState: task.State(s.ExpectedState),
		NodeName:      s.NodeName,
		OwnerUUID:     s.OwnerUUID,
		CreatedAt:     s.CreatedAt,
	}
}

func toBson(s *stack.Stack) StackBson {
	return StackBson{
		UUID:          s.UUID,
		Name:          s.Name,
		Slug:          s.Slug,
		Runtime:       string(s.Runtime),
		ExpectedState: int(s.ExpectedState),
		NodeName:      s.NodeName,
		OwnerUUID:     s.OwnerUUID,
		CreatedAt:     s.CreatedAt,
	}
}
