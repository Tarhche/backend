package roles

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/role"
)

type MockRolesRepository struct {
	mock.Mock
}

var _ role.Repository = &MockRolesRepository{}

func (r *MockRolesRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]role.Role, error) {
	args := r.Mock.Called(ctx, offset, limit)

	if a, ok := args.Get(0).([]role.Role); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockRolesRepository) GetByUUIDs(ctx context.Context, UUIDs []string) ([]role.Role, error) {
	args := r.Called(ctx, UUIDs)

	if c, ok := args.Get(0).([]role.Role); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockRolesRepository) GetOne(ctx context.Context, UUID string) (role.Role, error) {
	args := r.Mock.Called(ctx, UUID)

	return args.Get(0).(role.Role), args.Error(1)
}

func (r *MockRolesRepository) Save(ctx context.Context, rl *role.Role) (uuid string, err error) {
	args := r.Mock.Called(ctx, rl)

	return args.String(0), args.Error(1)
}

func (r *MockRolesRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Mock.Called(ctx, UUID)

	return args.Error(0)
}

func (r *MockRolesRepository) Count(ctx context.Context) (uint, error) {
	args := r.Mock.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockRolesRepository) UserHasPermission(ctx context.Context, userUUID string, permission string) (bool, error) {
	args := r.Mock.Called(ctx, userUUID, permission)

	return args.Bool(0), args.Error(1)
}

func (r *MockRolesRepository) GetByUserUUID(ctx context.Context, userUUID string) ([]role.Role, error) {
	args := r.Mock.Called(ctx, userUUID)

	if a, ok := args.Get(0).([]role.Role); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}
