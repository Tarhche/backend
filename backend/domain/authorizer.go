package domain

import (
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/stretchr/testify/mock"
)

type Authorizer interface {
	Authorize(userUUID string, permission string) (bool, error)
}

type RoleBasedAccessControl struct {
	roleRepository role.Repository
}

var _ Authorizer = &RoleBasedAccessControl{}

func NewRoleBasedAccessControl(roleRepository role.Repository) *RoleBasedAccessControl {
	return &RoleBasedAccessControl{
		roleRepository: roleRepository,
	}
}

func (a *RoleBasedAccessControl) Authorize(userUUID string, permission string) (bool, error) {
	hasPermission, err := a.roleRepository.UserHasPermission(userUUID, permission)
	if err != nil {
		return false, err
	}

	return hasPermission, nil
}

type MockAuthorizer struct {
	mock.Mock
}

var _ Authorizer = &MockAuthorizer{}

func (a *MockAuthorizer) Authorize(userUUID string, permission string) (bool, error) {
	args := a.Called(userUUID, permission)

	return args.Bool(0), args.Error(1)
}
