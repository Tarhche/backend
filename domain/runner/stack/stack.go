// Package stack groups the containers that make up one application. The
// services of a stack share a private network and are scheduled together onto
// one node, so they reach each other by service name the way a compose file
// expects.
package stack

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Stack is a set of containers run and managed as one thing.
//
// A stack holds no spec of its own: each of its services is a task, and the
// stack is what those tasks belong to. It is immutable, like the containers in
// it — there is no update, only run and remove.
type Stack struct {
	UUID string
	Name string
	Slug string

	// ExpectedState is what the stack was asked to be. It is kept here rather
	// than read off the services because a stack is asked for as a whole: while
	// a command is still reaching them, what they say between them is neither
	// one thing nor the other.
	ExpectedState task.State

	NodeName  string
	OwnerUUID string
	CreatedAt time.Time
}

// Repository stores stacks.
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Stack, error)

	// GetAllByOwner is the same listing, of what one person asked for.
	GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Stack, error)
	CountByOwner(ctx context.Context, ownerUUID string) (uint, error)
	GetOne(ctx context.Context, UUID string) (Stack, error)

	// GetOneByOwner is the same, of one person's own. A stack that is not
	// theirs is not there as far as they are concerned.
	GetOneByOwner(ctx context.Context, ownerUUID string, UUID string) (Stack, error)
	GetOneBySlug(ctx context.Context, slug string) (Stack, error)
	Save(ctx context.Context, s *Stack) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}
