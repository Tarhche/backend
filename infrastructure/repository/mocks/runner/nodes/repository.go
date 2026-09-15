package nodes

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type MockNodesRepository struct {
	mock.Mock
}

var _ node.Repository = &MockNodesRepository{}

func (r *MockNodesRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]node.Node, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if n, ok := args.Get(0).([]node.Node); ok {
		return n, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockNodesRepository) GetOne(ctx context.Context, name string) (node.Node, error) {
	args := r.Mock.Called(ctx, name)

	return args.Get(0).(node.Node), args.Error(1)
}

func (r *MockNodesRepository) Save(ctx context.Context, n *node.Node) (string, error) {
	args := r.Mock.Called(ctx, n)

	return args.String(0), args.Error(1)
}

func (r *MockNodesRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}
