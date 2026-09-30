package controlplane

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Page is one page of a listing.
type Page[T any] struct {
	Items       []T
	TotalPages  uint
	CurrentPage uint
}

// TaskSpec is one task to run.
type TaskSpec struct {
	Name string

	// Service is the task's specification in a compose service's shape.
	// It travels as it was written, so the runner reads it in exactly one
	// place and the dashboard does not have to understand it.
	Service any
}

// StackSpec is a set of services to run together.
type StackSpec struct {
	Name string

	// Services are the stack's services in a compose file's shape, keyed by
	// service name.
	Services any
}

// Client is the runner.
type Client interface {
	// Tasks is a page of the tasks the runner holds. An owner narrows it to
	// that person's own; empty is everybody's, including the ones nobody
	// owns.
	Tasks(ctx context.Context, ownerUUID string, page uint) (Page[task.Task], error)
	Task(ctx context.Context, uuid string) (task.Task, error)

	// TaskOf is one of somebody's own tasks. One that is not theirs
	// is not there as far as they are concerned, and is reported missing.
	TaskOf(ctx context.Context, ownerUUID string, uuid string) (task.Task, error)
	RunTask(ctx context.Context, spec TaskSpec, ownerUUID string) (task.Task, error)
	StopTask(ctx context.Context, uuid string) error
	KillTask(ctx context.Context, uuid string) error
	RestartTask(ctx context.Context, uuid string) error
	DeleteTask(ctx context.Context, uuid string) error

	TaskLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]task.Log, error)

	// Stacks is a page of the stacks the runner holds, narrowed the same way.
	Stacks(ctx context.Context, ownerUUID string, page uint) (Page[Stack], error)

	Stack(ctx context.Context, uuid string) (Stack, error)

	// StackOf is one of somebody's own stacks, reported missing when it is not
	// theirs.
	StackOf(ctx context.Context, ownerUUID string, uuid string) (Stack, error)
	RunStack(ctx context.Context, spec StackSpec, ownerUUID string) (Stack, error)
	StopStack(ctx context.Context, uuid string) error
	KillStack(ctx context.Context, uuid string) error
	RestartStack(ctx context.Context, uuid string) error
	DeleteStack(ctx context.Context, uuid string) error
}

// Stack is a stack together with the services in it, which is how the runner
// reports one: a stack's state is read off its services, so the two always
// travel together.
type Stack struct {
	stack.Stack

	// State is what the stack is, read off its services; ExpectedState is what
	// it was asked to be. They differ while a command is still reaching them.
	State         task.State
	ExpectedState task.State

	Services []task.Task
}
