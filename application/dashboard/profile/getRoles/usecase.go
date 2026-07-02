package getRoles

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

func (uc *UseCase) Execute(ctx context.Context, UUID string) (*Response, error) {
	roles, err := uc.roleRepository.GetByUserUUID(ctx, UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(roles), nil
}
