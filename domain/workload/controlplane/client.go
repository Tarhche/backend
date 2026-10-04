package controlplane

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Page is one page of a listing.
type Page[T any] struct {
	Items       []T
	TotalPages  uint
	CurrentPage uint
}

// Client is the workload.
type Client interface {
	// Task is one task the workload holds, whoever owns it. The code runner
	// reads one back to make sure it is its own before it takes it away.
	Task(ctx context.Context, uuid string) (task.Task, error)

	// DeleteTask removes a task whether or not it is still running: a delete
	// is a request to have it gone.
	DeleteTask(ctx context.Context, uuid string) error
}
