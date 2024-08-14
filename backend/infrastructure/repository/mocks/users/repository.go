package users

import (
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/stretchr/testify/mock"
)

type MockUsersRepository struct {
	mock.Mock
}

var _ user.Repository = &MockUsersRepository{}

func (r *MockUsersRepository) GetAll(offset uint, limit uint) ([]user.User, error) {
	args := r.Called(offset, limit)

	if c, ok := args.Get(0).([]user.User); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockUsersRepository) GetByUUIDs(UUIDs []string) ([]user.User, error) {
	args := r.Called(UUIDs)

	if c, ok := args.Get(0).([]user.User); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockUsersRepository) GetOne(UUID string) (user.User, error) {
	args := r.Called(UUID)

	return args.Get(0).(user.User), args.Error(1)
}

func (r *MockUsersRepository) GetOneByIdentity(username string) (user.User, error) {
	args := r.Called(username)

	return args.Get(0).(user.User), args.Error(1)
}

func (r *MockUsersRepository) Save(u *user.User) (uuid string, err error) {
	args := r.Called(u)

	return args.String(0), args.Error(1)
}

func (r *MockUsersRepository) Delete(UUID string) error {
	args := r.Called(UUID)

	return args.Error(0)
}

func (r *MockUsersRepository) Count() (uint, error) {
	args := r.Called()

	return args.Get(0).(uint), args.Error(1)
}
