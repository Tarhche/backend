package updaterole

import (
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
)

type UseCase struct {
	roleRepository       role.Repository
	permissionRepository permission.Repository
}

func NewUseCase(roleRepository role.Repository, permissionRepository permission.Repository) *UseCase {
	return &UseCase{
		roleRepository:       roleRepository,
		permissionRepository: permissionRepository,
	}
}

func (uc *UseCase) UpdateRole(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	if permissions, err := uc.permissionRepository.Get(request.Permissions); err != nil {
		return nil, err
	} else if len(permissions) != len(request.Permissions) {
		return &Response{
			ValidationErrors: validationErrors{
				"permissions": "one or more of permissions not exist",
			},
		}, nil
	}

	r := role.Role{
		UUID:        request.UUID,
		Name:        request.Name,
		Description: request.Description,
		Permissions: request.Permissions,
		UserUUIDs:   request.UserUUIDs,
	}

	if _, err := uc.roleRepository.Save(&r); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
