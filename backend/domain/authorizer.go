package domain

import (
	"github.com/khanzadimahdi/testproject/domain/role"
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
