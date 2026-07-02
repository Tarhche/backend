package getpermissions

import (
	"context"

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

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	items := uc.permissionRepository.GetAll(ctx)

	return NewResponse(items), nil
}
