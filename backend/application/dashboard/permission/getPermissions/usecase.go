package getpermissions

import (
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type UseCase struct {
	permissionRepository permission.Repository
}

func NewUseCase(permissionRepository permission.Repository) *UseCase {
	return &UseCase{
		permissionRepository: permissionRepository,
	}
}

func (uc *UseCase) GetPermissions() (*Response, error) {
	items := uc.permissionRepository.GetAll()

	return NewResponse(items), nil
}
