package stacks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
)

type MockStacksRepository struct {
	mock.Mock
}

var _ stack.Repository = &MockStacksRepository{}

func (r *MockStacksRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]stack.Stack, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if s, ok := args.Get(0).([]stack.Stack); ok {
		return s, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockStacksRepository) GetOne(ctx context.Context, UUID string) (stack.Stack, error) {
	args := r.Mock.Called(ctx, UUID)

	return args.Get(0).(stack.Stack), args.Error(1)
}

func (r *MockStacksRepository) GetOneBySlug(ctx context.Context, slug string) (stack.Stack, error) {
	args := r.Mock.Called(ctx, slug)

	return args.Get(0).(stack.Stack), args.Error(1)
}

func (r *MockStacksRepository) Save(ctx context.Context, s *stack.Stack) (string, error) {
	args := r.Mock.Called(ctx, s)

	return args.String(0), args.Error(1)
}

func (r *MockStacksRepository) Delete(ctx context.Context, UUID string) error {
	return r.Mock.Called(ctx, UUID).Error(0)
}

func (r *MockStacksRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}
