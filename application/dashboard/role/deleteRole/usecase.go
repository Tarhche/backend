package deleterole

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/role"
)

type UseCase struct {
	roleRepository role.Repository
}

func NewUseCase(roleRepository role.Repository) *UseCase {
	return &UseCase{
		roleRepository: roleRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.roleRepository.Delete(ctx, request.RoleUUID)
}
