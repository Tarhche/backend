package stacks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
)

type StackBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Name      string    `bson:"name"`
	Slug      string    `bson:"slug"`
	NodeName  string    `bson:"node_name,omitempty"`
	OwnerUUID string    `bson:"owner_uuid"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
}

func toStack(s *StackBson) stack.Stack {
	return stack.Stack{
		UUID:      s.UUID,
		Name:      s.Name,
		Slug:      s.Slug,
		NodeName:  s.NodeName,
		OwnerUUID: s.OwnerUUID,
		CreatedAt: s.CreatedAt,
	}
}

func toBson(s *stack.Stack) StackBson {
	return StackBson{
		UUID:      s.UUID,
		Name:      s.Name,
		Slug:      s.Slug,
		NodeName:  s.NodeName,
		OwnerUUID: s.OwnerUUID,
		CreatedAt: s.CreatedAt,
	}
}
