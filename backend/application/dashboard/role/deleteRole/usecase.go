package deleterole

import (
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

func (uc *UseCase) Execute(request *Request) error {
	return uc.roleRepository.Delete(request.RoleUUID)
}
