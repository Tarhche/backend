package users

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/user"
)

type MockUsersRepository struct {
	mock.Mock
}

var _ user.Repository = &MockUsersRepository{}

func (r *MockUsersRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]user.User, error) {
	args := r.Called(ctx, offset, limit)

	if c, ok := args.Get(0).([]user.User); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockUsersRepository) GetByUUIDs(ctx context.Context, UUIDs []string) ([]user.User, error) {
	args := r.Called(ctx, UUIDs)

	if c, ok := args.Get(0).([]user.User); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockUsersRepository) GetOne(ctx context.Context, UUID string) (user.User, error) {
	args := r.Called(ctx, UUID)

	return args.Get(0).(user.User), args.Error(1)
}

func (r *MockUsersRepository) GetOneByIdentity(ctx context.Context, username string) (user.User, error) {
	args := r.Called(ctx, username)

	return args.Get(0).(user.User), args.Error(1)
}

func (r *MockUsersRepository) Save(ctx context.Context, u *user.User) (uuid string, err error) {
	args := r.Called(ctx, u)

	return args.String(0), args.Error(1)
}

func (r *MockUsersRepository) Delete(ctx context.Context, UUID string) error {
	args := r.Called(ctx, UUID)

	return args.Error(0)
}

func (r *MockUsersRepository) Count(ctx context.Context) (uint, error) {
	args := r.Called(ctx)

	return args.Get(0).(uint), args.Error(1)
}
