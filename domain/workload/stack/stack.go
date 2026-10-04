// Package stack is a compose project deployed into a Docker VM.
//
// The workload keeps the record and the YAML; docker compose, inside the VM,
// does the rest. The containers a stack has are whichever ones compose labelled
// with its project, read from the VM's dockerd whenever they are asked for and
// never stored.
package stack

import (
	"context"
	"time"
)

// Stack is one compose project.
type Stack struct {
	UUID      string
	Name      string
	OwnerUUID string

	// VMUUID is the Docker VM the stack is deployed into.
	VMUUID string

	// Slug is the compose project's name: unique, lowercase letters, digits
	// and dashes. What the compose file calls its project is ignored.
	Slug string

	// Compose is the YAML as it was given. A stack is not edited: there is no
	// update, only deploy and remove.
	Compose string

	// ExpectedState is what the stack was asked to be, Running or Stopped.
	// State is what the last compose command left it as.
	ExpectedState State
	State         State

	Reason string

	// Output is the tail of what the last compose command printed, which is
	// how somebody finds out why a service did not come up.
	Output string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MaxOutput is the most of a compose command's output a stack keeps, in bytes,
// counted from the end: the last lines are the ones that say what went wrong.
const MaxOutput = 16 << 10

// Action is a compose command run on a stack.
type Action string

const (
	// ActionUp deploys the project, creating and starting what it needs and
	// removing what it no longer has.
	ActionUp Action = "up"

	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"

	// ActionDown removes the project's containers and networks, and its
	// volumes when that is asked for too.
	ActionDown Action = "down"
)

// IsValid reports whether a is one of the known actions.
func (a Action) IsValid() bool {
	switch a {
	case ActionUp, ActionStart, ActionStop, ActionRestart, ActionDown:
		return true
	default:
		return false
	}
}

func (a Action) String() string {
	return string(a)
}

// Repository stores stacks.
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Stack, error)

	// GetAllByOwner is the same listing, of one person's own.
	GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Stack, error)

	// GetAllByVM is every stack deployed into one VM.
	GetAllByVM(ctx context.Context, vmUUID string) ([]Stack, error)

	CountByOwner(ctx context.Context, ownerUUID string) (uint, error)
	Count(ctx context.Context) (uint, error)

	GetOne(ctx context.Context, uuid string) (Stack, error)

	// GetOneByOwner is one of somebody's own. A stack that is not theirs is
	// not there as far as they are concerned.
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (Stack, error)

	// GetOneBySlug finds a stack by its compose project.
	GetOneBySlug(ctx context.Context, slug string) (Stack, error)

	Save(ctx context.Context, s *Stack) (uuid string, err error)
	Delete(ctx context.Context, uuid string) error
}
