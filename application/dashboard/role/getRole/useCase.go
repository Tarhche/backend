package getrole

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
	a, err := uc.roleRepository.GetOne(ctx, UUID)
	if err != nil {
		return nil, err
	}

	return NewResponse(a), nil
}
