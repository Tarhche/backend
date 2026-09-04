// Package manager is the runner as the rest of the application sees it: what
// can be asked of it, in the terms the domain already has.
//
// The blog's dashboard does not schedule containers itself. It authenticates
// the person asking, decides whether they may, and passes the request on — so
// there is one place that owns a container's lifecycle rather than two that can
// disagree about it.
package manager

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Page is one page of a listing.
type Page[T any] struct {
	Items       []T
	TotalPages  uint
	CurrentPage uint
}

// ContainerSpec is one container to run.
type ContainerSpec struct {
	Name string

	// Service is the container's specification in a compose service's shape.
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

// Attachment is a command running inside a container: reading takes its output,
// writing feeds its input, and closing ends it.
type Attachment interface {
	io.ReadWriteCloser

	Resize(ctx context.Context, rows uint, cols uint) error
}

// ContainerChange is what became of one container: the container as it is now,
// or, when it is gone, the uuid of the one that was removed.
type ContainerChange struct {
	UUID      string
	Deleted   bool
	Container task.Task
}

// ContainerStream is what happens to the containers the runner holds, as it
// happens.
type ContainerStream interface {
	// Next blocks until the next change arrives. It reports io.EOF when the
	// stream ends.
	Next(ctx context.Context) (ContainerChange, error)

	io.Closer
}

// StackChange is what became of one stack: the stack as it is now, together
// with the services its state is read off, or, when it is gone, the uuid of the
// one that was removed.
type StackChange struct {
	UUID    string
	Deleted bool
	Stack   Stack
}

// StackStream is what happens to the stacks the runner holds, as it happens.
type StackStream interface {
	// Next blocks until the next change arrives. It reports io.EOF when the
	// stream ends.
	Next(ctx context.Context) (StackChange, error)

	io.Closer
}

// LogStream is a container's output as it is written.
type LogStream interface {
	// Next blocks until the next line arrives. It reports io.EOF when the
	// stream ends.
	Next(ctx context.Context) (container.Log, error)

	io.Closer
}

// Client is the runner.
type Client interface {
	// Containers is a page of the containers the runner holds. An owner
	// narrows it to that person's own; empty is everybody's, including the
	// ones nobody owns.
	Containers(ctx context.Context, ownerUUID string, page uint) (Page[task.Task], error)
	Container(ctx context.Context, uuid string) (task.Task, error)
	RunContainer(ctx context.Context, spec ContainerSpec, ownerUUID string) (task.Task, error)
	StopContainer(ctx context.Context, uuid string) error
	KillContainer(ctx context.Context, uuid string) error
	RestartContainer(ctx context.Context, uuid string) error
	DeleteContainer(ctx context.Context, uuid string) error

	// WatchContainers follows what happens to every container the runner
	// holds, so a listing of them can be kept as it is rather than asked for
	// again.
	WatchContainers(ctx context.Context) (ContainerStream, error)

	ContainerLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]container.Log, error)
	FollowContainerLogs(ctx context.Context, uuid string, after time.Time) (LogStream, error)
	AttachContainer(ctx context.Context, uuid string, command []string) (Attachment, error)

	// Stacks is a page of the stacks the runner holds, narrowed the same way.
	Stacks(ctx context.Context, ownerUUID string, page uint) (Page[Stack], error)

	// WatchStacks follows what happens to every stack the runner holds. A
	// stack's state is read off its services, so it changes whenever one of
	// them does.
	WatchStacks(ctx context.Context) (StackStream, error)
	Stack(ctx context.Context, uuid string) (Stack, error)
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
