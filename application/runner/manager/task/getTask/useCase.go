package gettask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase retrieves a task
type UseCase struct {
	taskRepository task.Repository

	// nodes is where the node holding a task answers, which is what a caller
	// needs to reach the container itself: a terminal and a live log are the
	// node's to serve.
	nodes node.Repository
}

// NewUseCase creates an instance of the UseCase
func NewUseCase(taskRepository task.Repository, nodes node.Repository) *UseCase {
	return &UseCase{
		taskRepository: taskRepository,
		nodes:          nodes,
	}
}

// Execute executes the use case
func (uc *UseCase) Execute(ctx context.Context, UUID string) (*Response, error) {
	t, err := uc.taskRepository.GetOne(ctx, UUID)
	if err != nil {
		return nil, err
	}

	return uc.present(ctx, t), nil
}

// ExecuteOwn is the same, of one person's own container. One that is not
// theirs is not there for them, which is what the caller passes on.
func (uc *UseCase) ExecuteOwn(ctx context.Context, ownerUUID string, UUID string) (*Response, error) {
	t, err := uc.taskRepository.GetOneByOwner(ctx, ownerUUID, UUID)
	if err != nil {
		return nil, err
	}

	return uc.present(ctx, t), nil
}

// present says where the node holding a task answers. A node that has gone,
// or one that never said, leaves the address empty: the container is reported
// as it is, and whoever wanted a terminal on it is told there is none.
func (uc *UseCase) present(ctx context.Context, t task.Task) *Response {
	response := NewResponse(t)

	if t.NodeName == "" {
		return response
	}

	n, err := uc.nodes.GetOne(ctx, t.NodeName)
	if err != nil {
		return response
	}

	response.NodeAPI = n.APIAddress

	return response
}
