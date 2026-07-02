package domain

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/stretchr/testify/mock"
)

type Authorizer interface {
	Authorize(ctx context.Context, userUUID string, permission string) (bool, error)
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

func (a *RoleBasedAccessControl) Authorize(ctx context.Context, userUUID string, permission string) (bool, error) {
	hasPermission, err := a.roleRepository.UserHasPermission(ctx, userUUID, permission)
	if err != nil {
		return false, err
	}

	return hasPermission, nil
}

type MockAuthorizer struct {
	mock.Mock
}

var _ Authorizer = &MockAuthorizer{}

func (a *MockAuthorizer) Authorize(ctx context.Context, userUUID string, permission string) (bool, error) {
	args := a.Called(ctx, userUUID, permission)

	return args.Bool(0), args.Error(1)
}
