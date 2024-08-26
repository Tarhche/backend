package roles

import (
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/role"
)

type MockRolesRepository struct {
	mock.Mock
}

func (r *MockRolesRepository) GetAll(offset uint, limit uint) ([]role.Role, error) {
	args := r.Mock.Called(offset, limit)

	if a, ok := args.Get(0).([]role.Role); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockRolesRepository) GetByUUIDs(UUIDs []string) ([]role.Role, error) {
	args := r.Called(UUIDs)

	if c, ok := args.Get(0).([]role.Role); ok {
		return c, args.Error(1)
	}

	return nil, args.Error(1)
}

func (r *MockRolesRepository) GetOne(UUID string) (role.Role, error) {
	args := r.Mock.Called(UUID)

	return args.Get(0).(role.Role), args.Error(1)
}

func (r *MockRolesRepository) Save(rl *role.Role) (uuid string, err error) {
	args := r.Mock.Called(rl)

	return args.String(0), args.Error(1)
}

func (r *MockRolesRepository) Delete(UUID string) error {
	args := r.Mock.Called(UUID)

	return args.Error(0)
}

func (r *MockRolesRepository) Count() (uint, error) {
	args := r.Mock.Called()

	return args.Get(0).(uint), args.Error(1)
}

func (r *MockRolesRepository) UserHasPermission(userUUID string, permission string) (bool, error) {
	args := r.Mock.Called(userUUID, permission)

	return args.Bool(0), args.Error(1)
}

func (r *MockRolesRepository) GetByUserUUID(userUUID string) ([]role.Role, error) {
	args := r.Mock.Called(userUUID)

	if a, ok := args.Get(0).([]role.Role); ok {
		return a, args.Error(1)
	}

	return nil, args.Error(1)
}
