package tasks

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type MockTasksRepository struct {
	mock.Mock
}

var _ task.Repository = &MockTasksRepository{}

func (r *MockTasksRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]task.Task, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if t, ok := args.Get(0).([]task.Task); ok {
		return t, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockTasksRepository) GetAllByStack(ctx context.Context, stackUUID string) ([]task.Task, error) {
	args := r.Mock.Called(ctx, stackUUID)

	if t, ok := args.Get(0).([]task.Task); ok {
		return t, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockTasksRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]task.Task, error) {
	args := r.Mock.Called(ctx, ownerUUID, offset, limit)

	if items, ok := args.Get(0).([]task.Task); ok {
		return items, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockTasksRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	args := r.Mock.Called(ctx, ownerUUID)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockTasksRepository) GetOne(ctx context.Context, UUID string) (task.Task, error) {
	args := r.Mock.Called(ctx, UUID)

	return args.Get(0).(task.Task), args.Error(1)
}

func (r *MockTasksRepository) GetOneBySlug(ctx context.Context, slug string) (task.Task, error) {
	args := r.Mock.Called(ctx, slug)

	return args.Get(0).(task.Task), args.Error(1)
}

func (r *MockTasksRepository) Save(ctx context.Context, t *task.Task) (string, error) {
	args := r.Mock.Called(ctx, t)

	return args.String(0), args.Error(1)
}

func (r *MockTasksRepository) Delete(ctx context.Context, UUID string) error {
	return r.Mock.Called(ctx, UUID).Error(0)
}

func (r *MockTasksRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}
