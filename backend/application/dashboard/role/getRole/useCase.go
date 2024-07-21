package getrole

import "github.com/khanzadimahdi/testproject/domain/role"

type UseCase struct {
	roleRepository role.Repository
}

func NewUseCase(roleRepository role.Repository) *UseCase {
	return &UseCase{
		roleRepository: roleRepository,
	}
}

func (uc *UseCase) GetRole(UUID string) (*Response, error) {
	a, err := uc.roleRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(a), nil
}
