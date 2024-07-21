package getroles

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

func (uc *UseCase) GetRoles(request *Request) (*Response, error) {
	totalArticles, err := uc.roleRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalArticles / limit

	if (totalPages * limit) != totalArticles {
		totalPages++
	}

	roles, err := uc.roleRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(roles, totalPages, currentPage), nil
}
