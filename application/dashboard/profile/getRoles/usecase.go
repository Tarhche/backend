package getRoles

import (
	"github.com/khanzadimahdi/testproject/domain/role"
)

const limit = 10

type UseCase struct {
	roleRepository role.Repository
}

func NewUseCase(roleRepository role.Repository) *UseCase {
	return &UseCase{
		roleRepository: roleRepository,
	}
}

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	roles, err := uc.roleRepository.GetByUserUUID(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(roles), nil
}
