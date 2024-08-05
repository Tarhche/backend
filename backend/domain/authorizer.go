package domain

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

type Authorizer interface {
	Authorize(userUUID string, permission string) (bool, error)
}

type RoleBasedAccessControl struct {
	elementRepository role.Repository
}

var _ Authorizer = &RoleBasedAccessControl{}

func NewRoleBasedAccessControl(elementRepository role.Repository) *RoleBasedAccessControl {
	return &RoleBasedAccessControl{
		elementRepository: elementRepository,
	}
}

func (a *RoleBasedAccessControl) Authorize(userUUID string, permission string) (bool, error) {
	hasPermission, err := a.elementRepository.UserHasPermission(userUUID, permission)
	if err != nil {
		return false, err
	}

	return hasPermission, nil
}
