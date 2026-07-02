package getroles

import (
	"context"

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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	totalRoles, err := uc.roleRepository.Count(ctx)
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalRoles / limit

	if (totalPages * limit) != totalRoles {
		totalPages++
	}

	roles, err := uc.roleRepository.GetAll(ctx, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(roles, totalPages, currentPage), nil
}
