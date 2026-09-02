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

// LogStream is a container's output as it is written.
type LogStream interface {
	// Next blocks until the next line arrives. It reports io.EOF when the
	// stream ends.
	Next(ctx context.Context) (container.Log, error)

	io.Closer
}

// Client is the runner.
type Client interface {
	Containers(ctx context.Context, page uint) (Page[task.Task], error)
	Container(ctx context.Context, uuid string) (task.Task, error)
	RunContainer(ctx context.Context, spec ContainerSpec, ownerUUID string) (task.Task, error)
	StopContainer(ctx context.Context, uuid string) error
	KillContainer(ctx context.Context, uuid string) error
	RestartContainer(ctx context.Context, uuid string) error
	DeleteContainer(ctx context.Context, uuid string) error

	ContainerLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]container.Log, error)
	FollowContainerLogs(ctx context.Context, uuid string, after time.Time) (LogStream, error)
	AttachContainer(ctx context.Context, uuid string, command []string) (Attachment, error)

	Stacks(ctx context.Context, page uint) (Page[Stack], error)
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

	State    task.State
	Services []task.Task
}
